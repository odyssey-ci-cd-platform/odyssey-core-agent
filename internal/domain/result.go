package domain

import "time"

// StepResult holds the output of a single step execution.
type StepResult struct {
	StepName string
	Stdout   string
	Stderr   string
	ExitCode ExitCode
	Duration time.Duration
	// Err carries an infrastructure fault (exec or env read failure) as
	// opposed to a process failure, which is a non-zero ExitCode.
	Err error
}

// Status returns Errored if the step hit an infrastructure fault, Passed if
// the process exited successfully, else Failed.
func (r StepResult) Status() Status {
	if r.Err != nil {
		return StatusErrored
	}
	if r.ExitCode == ExitSuccess {
		return StatusPassed
	}
	return StatusFailed
}

// JobResult holds the results of all steps within a job.
type JobResult struct {
	JobName     string
	StepResults []StepResult
	SetupErr    error
}

// Status returns:
//   - StatusErrored if the job's setup failed or any step hit an
//     infrastructure fault
//   - StatusPending if there are no steps and no setup error
//   - StatusFailed if any step's process exited non-zero
//   - StatusPassed if all steps passed
func (r JobResult) Status() Status {
	if r.SetupErr != nil {
		return StatusErrored
	}
	if len(r.StepResults) == 0 {
		return StatusPending
	}
	worst := StatusPassed
	for _, sr := range r.StepResults {
		worst = worstStatus(worst, sr.Status())
	}
	return worst
}

// Duration returns time taken (time.Duration) for the job to complete
func (r JobResult) Duration() time.Duration {
	if len(r.StepResults) == 0 {
		return 0
	}
	jobDuration := time.Duration(0)
	for _, sr := range r.StepResults {
		jobDuration += sr.Duration
	}
	return jobDuration
}

// StageResult holds the results of all jobs within a stage.
type StageResult struct {
	StageName  string
	JobResults []JobResult
}

// Status returns the worst status among all jobs, in precedence order
// Errored > Failed > Pending > Passed, or Pending if there are no jobs.
func (r StageResult) Status() Status {
	if len(r.JobResults) == 0 {
		return StatusPending
	}
	worst := StatusPassed
	for _, jr := range r.JobResults {
		worst = worstStatus(worst, jr.Status())
	}
	return worst
}

// Duration returns time taken (time.Duration) for the stage to complete
func (r StageResult) Duration() time.Duration {
	if len(r.JobResults) == 0 {
		return 0
	}
	stageDuration := time.Duration(0)
	for _, jr := range r.JobResults {
		stageDuration += jr.Duration()
	}
	return stageDuration
}

// PipelineResult holds the results of all stages within a pipeline.
type PipelineResult struct {
	// RunID identifies the run that produced this result (AUD-012).
	RunID        string
	PipelineName string
	StageResults []StageResult
}

// Status returns the worst status among all stages, in precedence order
// Errored > Failed > Pending > Passed, or Pending if there are no stages.
func (r PipelineResult) Status() Status {
	if len(r.StageResults) == 0 {
		return StatusPending
	}
	worst := StatusPassed
	for _, sr := range r.StageResults {
		worst = worstStatus(worst, sr.Status())
	}
	return worst
}

// Duration returns time taken (time.Duration) for the Pipeline to complete
func (r PipelineResult) Duration() time.Duration {
	if len(r.StageResults) == 0 {
		return 0
	}
	pipelineDuration := time.Duration(0)
	for _, sr := range r.StageResults {
		pipelineDuration += sr.Duration()
	}
	return pipelineDuration
}

// statusRank gives each Status a precedence for aggregation purposes:
// Error is worst, then Failed, then Pending, then Passed is best.
func statusRank(s Status) int {
	switch s {
	case StatusErrored:
		return 3
	case StatusFailed:
		return 2
	case StatusPending:
		return 1
	default: // StatusPassed
		return 0
	}
}

// worstStatus returns whichever of a, b ranks worse per statusRank.
func worstStatus(a, b Status) Status {
	if statusRank(b) > statusRank(a) {
		return b
	}
	return a
}
