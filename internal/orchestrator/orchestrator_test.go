package orchestrator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeberg.org/odyssey/odyssey-core-agent/internal/domain"
	"codeberg.org/odyssey/odyssey-core-agent/internal/orchestrator"
	"codeberg.org/odyssey/odyssey-core-agent/internal/runner"
)

// fakeRunner is a Runner that returns pre-configured results keyed by job name.
type fakeRunner struct {
	results map[string]domain.JobResult
	errs    map[string]error

	mu    sync.Mutex
	calls []string // records job names in the order Run() was called
}

func (f *fakeRunner) Run(_ context.Context, job domain.Job, _ string) (domain.JobResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, job.Name)
	f.mu.Unlock()
	return f.results[job.Name], f.errs[job.Name]
}

// blockingRunner blocks each Run() call until the test releases it.
// Used to verify that jobs within a stage start concurrently.
type blockingRunner struct {
	entered *sync.WaitGroup // signaled when Run() is entered
	release <-chan struct{} // closed by the test to unblock
	results map[string]domain.JobResult
}

func (b *blockingRunner) Run(_ context.Context, job domain.Job, _ string) (domain.JobResult, error) {
	b.entered.Done()
	<-b.release
	return b.results[job.Name], nil
}

// newPassedJob returns a JobResult with a single passed step.
func newPassedJob(name string) domain.JobResult {
	return domain.JobResult{
		JobName: name,
		StepResults: []domain.StepResult{
			{StepName: "step", ExitCode: domain.ExitSuccess},
		},
	}
}

// newFailedJob returns a JobResult with a single failed step.
func newFailedJob(name string) domain.JobResult {
	return domain.JobResult{
		JobName: name,
		StepResults: []domain.StepResult{
			{StepName: "step", ExitCode: domain.ExitFailure},
		},
	}
}

// newErroredJob returns a JobResult whose setup failed.
func newErroredJob(name string) domain.JobResult {
	return domain.JobResult{
		JobName:  name,
		SetupErr: &stubError{"setup failed"},
	}
}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }

// simpleJob is a helper to build a domain.Job with minimal boilerplate.
func simpleJob(name string) domain.Job {
	return domain.Job{
		Name:  name,
		Image: "alpine:latest",
		Steps: []domain.Step{{Name: "s", Run: "echo " + name}},
	}
}

func TestOrchestratorSingleStageSingleJob(t *testing.T) {
	r := &fakeRunner{
		results: map[string]domain.JobResult{
			"build": newPassedJob("build"),
		},
		errs: map[string]error{},
	}
	o := orchestrator.New(r, nil)

	pipeline := domain.Pipeline{
		Name: "ci",
		Stages: []domain.Stage{
			{
				Name: "build-stage",
				Jobs: []domain.Job{simpleJob("build")},
			},
		},
	}

	result, err := o.Run(context.Background(), pipeline, "/tmp")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if result.PipelineName != "ci" {
		t.Errorf("PipelineName = %q, want %q", result.PipelineName, "ci")
	}
	if len(result.StageResults) != 1 {
		t.Fatalf("expected 1 stage result, got %d", len(result.StageResults))
	}

	sr := result.StageResults[0]
	if sr.StageName != "build-stage" {
		t.Errorf("StageName = %q, want %q", sr.StageName, "build-stage")
	}
	if len(sr.JobResults) != 1 {
		t.Fatalf("expected 1 job result, got %d", len(sr.JobResults))
	}
	if sr.JobResults[0].JobName != "build" {
		t.Errorf("JobName = %q, want %q", sr.JobResults[0].JobName, "build")
	}
	if result.Status() != domain.StatusPassed {
		t.Errorf("Status() = %v, want %v", result.Status(), domain.StatusPassed)
	}
}

func TestOrchestratorSingleStageMultipleJobs(t *testing.T) {
	r := &fakeRunner{
		results: map[string]domain.JobResult{
			"lint":   newPassedJob("lint"),
			"test":   newPassedJob("test"),
			"deploy": newPassedJob("deploy"),
		},
		errs: map[string]error{},
	}
	o := orchestrator.New(r, nil)

	pipeline := domain.Pipeline{
		Name: "ci",
		Stages: []domain.Stage{
			{
				Name: "all",
				Jobs: []domain.Job{
					simpleJob("lint"),
					simpleJob("test"),
					simpleJob("deploy"),
				},
			},
		},
	}

	result, err := o.Run(context.Background(), pipeline, "/tmp")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if len(result.StageResults) != 1 {
		t.Fatalf("expected 1 stage result, got %d", len(result.StageResults))
	}

	jobs := result.StageResults[0].JobResults
	if len(jobs) != 3 {
		t.Fatalf("expected 3 job results, got %d", len(jobs))
	}

	names := make(map[string]bool)
	for _, jr := range jobs {
		names[jr.JobName] = true
	}
	for _, want := range []string{"lint", "test", "deploy"} {
		if !names[want] {
			t.Errorf("missing job result for %q", want)
		}
	}

	if result.Status() != domain.StatusPassed {
		t.Errorf("Status() = %v, want %v", result.Status(), domain.StatusPassed)
	}
}

func TestOrchestratorMultipleStages(t *testing.T) {
	r := &fakeRunner{
		results: map[string]domain.JobResult{
			"build":  newPassedJob("build"),
			"test":   newPassedJob("test"),
			"deploy": newPassedJob("deploy"),
		},
		errs: map[string]error{},
	}
	o := orchestrator.New(r, nil)

	pipeline := domain.Pipeline{
		Name: "full-ci",
		Stages: []domain.Stage{
			{Name: "build-stage", Jobs: []domain.Job{simpleJob("build")}},
			{Name: "test-stage", Jobs: []domain.Job{simpleJob("test")}},
			{Name: "deploy-stage", Jobs: []domain.Job{simpleJob("deploy")}},
		},
	}

	result, err := o.Run(context.Background(), pipeline, "/tmp")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if len(result.StageResults) != 3 {
		t.Fatalf("expected 3 stage results, got %d", len(result.StageResults))
	}

	wantStages := []string{"build-stage", "test-stage", "deploy-stage"}
	for i, want := range wantStages {
		if result.StageResults[i].StageName != want {
			t.Errorf("stage %d: name = %q, want %q", i, result.StageResults[i].StageName, want)
		}
	}

	// Verify the runner's call order: stages execute sequentially, so
	// calls[0]="build", calls[1]="test", calls[2]="deploy".
	r.mu.Lock()
	calls := make([]string, len(r.calls))
	copy(calls, r.calls)
	r.mu.Unlock()

	if len(calls) != 3 {
		t.Fatalf("expected 3 Run() calls, got %d", len(calls))
	}
	wantCalls := []string{"build", "test", "deploy"}
	for i, want := range wantCalls {
		if calls[i] != want {
			t.Errorf("call %d: got %q, want %q", i, calls[i], want)
		}
	}

	if result.Status() != domain.StatusPassed {
		t.Errorf("Status() = %v, want %v", result.Status(), domain.StatusPassed)
	}
}

func TestOrchestratorMixedStatuses(t *testing.T) {
	r := &fakeRunner{
		results: map[string]domain.JobResult{
			"good": newPassedJob("good"),
			"bad":  newFailedJob("bad"),
			"dead": newErroredJob("dead"),
		},
		errs: map[string]error{
			"dead": &stubError{"setup failed"}, // Runner returns the error too
		},
	}
	o := orchestrator.New(r, nil)

	pipeline := domain.Pipeline{
		Name: "mixed-ci",
		Stages: []domain.Stage{
			{
				Name: "stage",
				Jobs: []domain.Job{
					simpleJob("good"),
					simpleJob("bad"),
					simpleJob("dead"),
				},
			},
		},
	}

	result, err := o.Run(context.Background(), pipeline, "/tmp")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// All three jobs ran even though one errored and one failed.
	if len(result.StageResults[0].JobResults) != 3 {
		t.Fatalf("expected 3 job results, got %d", len(result.StageResults[0].JobResults))
	}

	// Status should be Errored (worst: Errored > Failed > Passed).
	if result.Status() != domain.StatusErrored {
		t.Errorf("Status() = %v, want %v", result.Status(), domain.StatusErrored)
	}
}

func TestOrchestratorEmptyPipeline(t *testing.T) {
	r := &fakeRunner{
		results: map[string]domain.JobResult{},
		errs:    map[string]error{},
	}
	o := orchestrator.New(r, nil)

	pipeline := domain.Pipeline{
		Name:   "empty",
		Stages: []domain.Stage{},
	}

	result, err := o.Run(context.Background(), pipeline, "/tmp")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if result.PipelineName != "empty" {
		t.Errorf("PipelineName = %q, want %q", result.PipelineName, "empty")
	}
	if len(result.StageResults) != 0 {
		t.Errorf("expected 0 stage results, got %d", len(result.StageResults))
	}
	if result.Status() != domain.StatusPending {
		t.Errorf("Status() = %v, want %v", result.Status(), domain.StatusPending)
	}
}

func TestOrchestratorJobsRunConcurrently(t *testing.T) {
	// Proves that jobs within a stage are launched concurrently, not
	// sequentially.  Each Run() call signals entered.Done() then blocks
	// on <-release.  The test waits for all three to enter, then closes
	// release to let them all finish.
	var entered sync.WaitGroup
	release := make(chan struct{})

	rb := &blockingRunner{
		entered: &entered,
		release: release,
		results: map[string]domain.JobResult{
			"a": newPassedJob("a"),
			"b": newPassedJob("b"),
			"c": newPassedJob("c"),
		},
	}
	o := orchestrator.New(rb, nil)

	pipeline := domain.Pipeline{
		Name: "concurrent",
		Stages: []domain.Stage{
			{
				Name: "stage",
				Jobs: []domain.Job{
					simpleJob("a"),
					simpleJob("b"),
					simpleJob("c"),
				},
			},
		},
	}

	entered.Add(3) // expect 3 Run() calls

	done := make(chan struct{})
	var result domain.PipelineResult
	go func() {
		result, _ = o.Run(context.Background(), pipeline, "/tmp")
		close(done)
	}()

	// Wait for all 3 jobs to enter Run().
	waitDone := make(chan struct{})
	go func() {
		entered.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// All three are inside Run() concurrently. Release them.
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for jobs to start — they may be running sequentially")
	}
	close(release)

	<-done

	if len(result.StageResults[0].JobResults) != 3 {
		t.Errorf("expected 3 job results, got %d", len(result.StageResults[0].JobResults))
	}
}

func TestOrchestratorRunnerErrorDoesNotBlockOtherJobs(t *testing.T) {
	// When one job's runner returns an error, sibling jobs must still run.
	// The orchestrator surfaces errors through JobResult.Status(), not by
	// aborting the stage.
	r := &fakeRunner{
		results: map[string]domain.JobResult{
			"good": newPassedJob("good"),
			"dead": newErroredJob("dead"),
		},
		errs: map[string]error{
			"dead": &stubError{"setup failed"},
		},
	}
	o := orchestrator.New(r, nil)

	pipeline := domain.Pipeline{
		Name: "error-test",
		Stages: []domain.Stage{
			{
				Name: "stage",
				Jobs: []domain.Job{
					simpleJob("dead"),
					simpleJob("good"),
				},
			},
		},
	}

	result, err := o.Run(context.Background(), pipeline, "/tmp")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Both jobs must have results, even though one errored.
	jobs := result.StageResults[0].JobResults
	if len(jobs) != 2 {
		t.Fatalf("expected 2 job results, got %d", len(jobs))
	}

	// The good job should still have passed.
	foundGood := false
	for _, jr := range jobs {
		if jr.JobName == "good" && jr.Status() == domain.StatusPassed {
			foundGood = true
		}
	}
	if !foundGood {
		t.Error("the 'good' job was not run or did not pass — errored sibling blocked it")
	}
}

// Compile-time check that our fakes satisfy the Runner interface.
var _ runner.Runner = (*fakeRunner)(nil)
var _ runner.Runner = (*blockingRunner)(nil)
