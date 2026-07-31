package domain

// Step is a single shell command within a job.
type Step struct {
	Name string
	Run  string
}

// Job is a collection of steps executed inside a container.
type Job struct {
	Name  string
	Image string
	Setup []string
	Steps []Step
	Env   map[string]string
}

// Stage is a named group of jobs that run in parallel.
type Stage struct {
	Name string
	Jobs []Job
}

// Pipeline is the top-level execution unit composed of ordered stages.
type Pipeline struct {
	Name   string
	Stages []Stage
}
