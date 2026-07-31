package domain

type Status struct {
	slug string
}

func (s Status) String() string {
	return s.slug
}

var (
	StatusUnknown = Status{""}
	StatusPending = Status{"pending"}
	StatusRunning = Status{"running"}
	StatusPassed  = Status{"passed"}
	StatusFailed  = Status{"failed"}
	StatusSkipped = Status{"skipped"}
	StatusErrored = Status{"errored"}
)

type ExitCode int

const (
	ExitSuccess ExitCode = iota
	ExitFailure
)
