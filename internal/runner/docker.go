package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/common"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const workDir = "/app"

// shortID truncates a Docker container ID to 12 characters for logging.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

type DockerRunner struct {
	client *client.Client
	logger *slog.Logger
}

func NewDockerRunner(logger *slog.Logger) (*DockerRunner, error) {
	c, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &DockerRunner{client: c, logger: logger}, nil
}

// loggerFromCtx returns the logger attached to ctx, falling back to the
// runner's own logger when none is present.
func (r *DockerRunner) loggerFromCtx(ctx context.Context) *slog.Logger {
	return common.LoggerFromContext(ctx, r.logger)
}

func (r *DockerRunner) Run(ctx context.Context, job domain.Job, projectPath string) (domain.JobResult, error) {
	jobResult := domain.JobResult{JobName: job.Name}

	// Pull image
	err := r.pullImage(ctx, job.Image)
	if err != nil {
		err = fmt.Errorf("failed to pull image %q: %w", job.Image, err)
		jobResult.SetupErr = err
		return jobResult, err
	}
	r.loggerFromCtx(ctx).Info("docker image pulled", "image", job.Image)

	// Create container
	containerID, err := r.createContainer(ctx, job, projectPath)
	if err != nil {
		err = fmt.Errorf("failed to create container: %w", err)
		jobResult.SetupErr = err
		return jobResult, err
	}
	r.loggerFromCtx(ctx).Info("container created", "containerID", shortID(containerID))

	defer func() {
		if err := r.removeContainer(ctx, containerID); err != nil {
			r.loggerFromCtx(ctx).Error("remove container failed", "containerID", shortID(containerID), "error", err)
		}
	}()

	// Start container
	if _, err := r.client.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		err = fmt.Errorf("failed to start container: %w", err)
		jobResult.SetupErr = err
		return jobResult, err
	}
	r.loggerFromCtx(ctx).Info("container started", "containerID", shortID(containerID))

	// Run setup commands. Reported via JobResult.SetupErr
	if err := r.runSetup(ctx, containerID, job.Setup); err != nil {
		err = fmt.Errorf("job setup failed: %w", err)
		jobResult.SetupErr = err
		return jobResult, err
	}
	r.loggerFromCtx(ctx).Info("setup commands ran", "containerID", shortID(containerID))

	// Run steps
	stepResults, stepRunErr := r.runSteps(ctx, containerID, job.Steps)
	if stepRunErr != nil {
		stepRunErr = fmt.Errorf("failed to run steps: %w", stepRunErr)
	}
	r.loggerFromCtx(ctx).Info("steps completed", "containerID", shortID(containerID))

	jobResult.StepResults = stepResults
	return jobResult, stepRunErr
}

// pullImage pulls the docker image and discards the response body.
func (r *DockerRunner) pullImage(ctx context.Context, img string) error {
	imagePullResponse, err := r.client.ImagePull(ctx, img, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer func() {
		pullImageRespCloseErr := imagePullResponse.Close()
		if pullImageRespCloseErr != nil {
			r.loggerFromCtx(ctx).Warn("failed to close image pull response", "error", pullImageRespCloseErr)
		}
	}()
	_, err = io.Copy(io.Discard, imagePullResponse)
	return err
}

// createContainer creates a container for the job without starting it.
// It also mounts the projectPath to the container.
func (r *DockerRunner) createContainer(ctx context.Context, job domain.Job, projectPath string) (string, error) {
	env := make([]string, 0, len(job.Env)+1)
	for k, v := range job.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	// Expose the env-file path so steps can export vars to later steps.
	env = append(env, fmt.Sprintf("%s=%s", envFileVar, envFilePath))

	resp, err := r.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      job.Image,
			Env:        env,
			WorkingDir: workDir,
			Cmd:        []string{"sh", "-c", "tail -f /dev/null"},
		},
	})
	if err != nil {
		return "", err
	}
	containerID := resp.ID
	if err := r.mountArchive(ctx, containerID, projectPath, workDir); err != nil {
		removeErr := r.removeContainer(ctx, containerID)
		if removeErr != nil {
			err = fmt.Errorf("%w; additionally, failed to remove container: %w", err, removeErr)
		}
		return "", fmt.Errorf("failed to mount project path to container: %w", err)
	}
	return containerID, nil
}

// removeContainer removes a container by its ID.
func (r *DockerRunner) removeContainer(ctx context.Context, containerID string) error {
	_, err := r.client.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	if err == nil {
		r.loggerFromCtx(ctx).Info("container removed", "containerID", shortID(containerID))
	}
	return err
}

// runSetup runs setup command for the Job.
func (r *DockerRunner) runSetup(ctx context.Context, containerID string, setupCmd []string) error {
	for _, command := range setupCmd {
		if _, _, _, err := r.execCommand(ctx, containerID, command, nil); err != nil {
			return err
		}
	}
	return nil
}

func (r *DockerRunner) runSteps(ctx context.Context, containerID string, steps []domain.Step) ([]domain.StepResult, error) {
	stepResults := make([]domain.StepResult, 0, len(steps))

	// Start each job with an empty env file so a step can append to it and
	// later steps can read it back, even if no earlier step wrote anything.
	if _, _, _, err := r.execCommand(ctx, containerID, ": > "+envFilePath, nil); err != nil {
		return stepResults, fmt.Errorf("failed to initialize env file: %w", err)
	}

	for _, step := range steps {
		// Vars exported by earlier steps, injected as this exec's env. Exec env
		// overrides the container's job-level env.
		exported, err := r.readExportedEnv(ctx, containerID)
		if err != nil {
			return stepResults, fmt.Errorf("failed to read exported env before step %q: %w", step.Name, err)
		}

		stepStartTime := time.Now()
		exitCode, stdout, stderr, runErr := r.execCommand(ctx, containerID, step.Run, envSlice(exported))
		stepResults = append(stepResults, domain.StepResult{
			StepName: step.Name,
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
			Duration: time.Since(stepStartTime),
		})

		if runErr != nil {
			return stepResults, runErr
		}

		// Surface which vars this step exported for later steps. Values are
		// omitted since they may hold secrets; a future UI can show them.
		after, err := r.readExportedEnv(ctx, containerID)
		if err != nil {
			return stepResults, fmt.Errorf("failed to read exported env after step %q: %w", step.Name, err)
		}
		if keys := newlyExportedKeys(exported, after); len(keys) > 0 {
			r.loggerFromCtx(ctx).Info("step exported env vars", "step", step.Name, "keys", keys)
		}
	}
	return stepResults, nil
}

// readExportedEnv reads the in-container env file and parses its KEY=value
// lines into a map. See parseEnvFile for the parsing rules.
func (r *DockerRunner) readExportedEnv(ctx context.Context, containerID string) (map[string]string, error) {
	_, stdout, _, err := r.execCommand(ctx, containerID, "cat "+envFilePath, nil)
	if err != nil {
		return nil, err
	}
	return parseEnvFile(stdout), nil
}

// execCommand runs a command inside a container and returns its exit code,
// stdout, stderr, and an error.
func (r *DockerRunner) execCommand(ctx context.Context, containerID string, command string, env []string) (domain.ExitCode, string, string, error) {
	execConfig := client.ExecCreateOptions{
		Cmd:          []string{"sh", "-c", command},
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
	}
	execCreateResult, err := r.client.ExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return domain.ExitFailure, "", "", err
	}

	execID := execCreateResult.ID
	resp, err := r.client.ExecAttach(ctx, execID, client.ExecAttachOptions{})
	if err != nil {
		return domain.ExitFailure, "", "", err
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return domain.ExitFailure, "", "", err
	}

	inspect, err := r.client.ExecInspect(ctx, execID, client.ExecInspectOptions{})
	if err != nil {
		return domain.ExitFailure, "", "", err
	}

	out, errOut := stdout.String(), stderr.String()
	if inspect.ExitCode != 0 {
		return domain.ExitFailure, out, errOut, fmt.Errorf("command exited with exit code %d", inspect.ExitCode)
	}
	return domain.ExitSuccess, out, errOut, nil
}

// mountArchive creates a tar of the project directory and copies it into destinationPath inside the container
func (r *DockerRunner) mountArchive(ctx context.Context, containerID string, projectPath string, destinationPath string) error {
	archive, err := common.CreateTar(projectPath)
	if err != nil {
		return err
	}
	if len(archive) == 0 {
		return fmt.Errorf("cannot mount an empty archive")
	}
	reader := bytes.NewReader(archive)
	if _, err := r.client.CopyToContainer(ctx, containerID, client.CopyToContainerOptions{
		DestinationPath: destinationPath,
		Content:         reader,
	}); err != nil {
		return err
	}
	return nil
}
