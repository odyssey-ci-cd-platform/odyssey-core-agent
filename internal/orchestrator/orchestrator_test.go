package orchestrator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/orchestrator"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/runner"
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

// blockingRunner blocks each Run() call until the test releases it, to verify that jobs within a stage start concurrently.
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

// fakeSink is an EventSink that records events in memory, or fails every
// publish when err is set.
type fakeSink struct {
	err error

	mu     sync.Mutex
	events []domain.Event
}

func (f *fakeSink) Publish(_ context.Context, event domain.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, event)
	return nil
}

func (f *fakeSink) recorded() []domain.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Event(nil), f.events...)
}

// eventsOfType returns the events whose Type matches typ.
func eventsOfType(events []domain.Event, typ string) []domain.Event {
	var out []domain.Event
	for _, e := range events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func TestOrchestratorEmitsLifecycleEvents(t *testing.T) {
	// Requirement (gh-52, ADR 0001): a pipeline run emits started/finished
	// events for the pipeline and for each job, with finished events
	// carrying the aggregated status.
	fake := &fakeSink{}
	r := &fakeRunner{results: map[string]domain.JobResult{
		"a": newPassedJob("a"),
		"b": newFailedJob("b"),
		"c": newErroredJob("c"),
	}}
	o := orchestrator.New(r, fake, nil)
	pipeline := domain.Pipeline{
		Name: "shop",
		Stages: []domain.Stage{
			{Name: "build", Jobs: []domain.Job{simpleJob("a"), simpleJob("b"), simpleJob("c")}},
		},
	}
	if _, err := o.Run(context.Background(), pipeline, "."); err != nil {
		t.Fatalf("Run: %v", err)
	}

	events := fake.recorded()
	if len(events) != 8 {
		t.Fatalf("got %d events, want 8: %+v", len(events), events)
	}
	first, last := events[0], events[len(events)-1]
	if first.Type != domain.EventPipelineStarted || first.Pipeline != "shop" {
		t.Errorf("first event = %+v, want pipeline started for shop", first)
	}
	if last.Type != domain.EventPipelineFinished || last.Pipeline != "shop" {
		t.Errorf("last event = %+v, want pipeline finished for shop", last)
	}
	if got := last.Payload["status"]; got != domain.StatusErrored.String() {
		t.Errorf("pipeline finished payload status = %q, want %q (Error > Failed precedence)", got, domain.StatusErrored.String())
	}

	wantStatus := map[string]domain.Status{
		"a": domain.StatusPassed,
		"b": domain.StatusFailed,
		"c": domain.StatusErrored,
	}
	for job, status := range wantStatus {
		started, finished := -1, -1
		for i, e := range events {
			if e.Job != job {
				continue
			}
			switch e.Type {
			case domain.EventJobStarted:
				started = i
			case domain.EventJobFinished:
				finished = i
				if got := e.Payload["status"]; got != status.String() {
					t.Errorf("job %s finished payload status = %q, want %q", job, got, status.String())
				}
			}
		}
		if started == -1 || finished == -1 {
			t.Errorf("job %s: missing started or finished event in %+v", job, events)
			continue
		}
		if started > finished {
			t.Errorf("job %s: started event at %d after finished event at %d", job, started, finished)
		}
	}
	if got := len(eventsOfType(events, domain.EventPipelineStarted)); got != 1 {
		t.Errorf("got %d pipeline started events, want 1", got)
	}
	if got := len(eventsOfType(events, domain.EventPipelineFinished)); got != 1 {
		t.Errorf("got %d pipeline finished events, want 1", got)
	}
}

func TestOrchestratorSinkErrorDoesNotFailRun(t *testing.T) {
	// Requirement (ADR 0001 graceful degradation): emission failures are
	// logged, never fatal — Run's outcome is unchanged when the bus is down.
	breaking := &fakeSink{err: &stubError{"bus down"}}
	r := &fakeRunner{results: map[string]domain.JobResult{"a": newPassedJob("a")}}
	pipeline := domain.Pipeline{
		Name: "shop",
		Stages: []domain.Stage{
			{Name: "build", Jobs: []domain.Job{simpleJob("a")}},
		},
	}
	result, err := orchestrator.New(r, breaking, nil).Run(context.Background(), pipeline, ".")
	if err != nil {
		t.Fatalf("Run with failing sink: %v", err)
	}
	if result.Status() != domain.StatusPassed {
		t.Errorf("status = %v, want passed despite failing sink", result.Status())
	}
}

func TestOrchestratorSingleStageSingleJob(t *testing.T) {
	r := &fakeRunner{
		results: map[string]domain.JobResult{
			"build": newPassedJob("build"),
		},
		errs: map[string]error{},
	}
	o := orchestrator.New(r, nil, nil)

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
	o := orchestrator.New(r, nil, nil)

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
	o := orchestrator.New(r, nil, nil)

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
	o := orchestrator.New(r, nil, nil)

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
	o := orchestrator.New(r, nil, nil)

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
	// Verifies jobs within a stage start concurrently, not sequentially.
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
	o := orchestrator.New(rb, nil, nil)

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

	waitDone := make(chan struct{})
	go func() {
		entered.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
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
	// A job's runner error must not block sibling jobs in the stage.
	r := &fakeRunner{
		results: map[string]domain.JobResult{
			"good": newPassedJob("good"),
			"dead": newErroredJob("dead"),
		},
		errs: map[string]error{
			"dead": &stubError{"setup failed"},
		},
	}
	o := orchestrator.New(r, nil, nil)

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
var (
	_ runner.Runner = (*fakeRunner)(nil)
	_ runner.Runner = (*blockingRunner)(nil)
)
