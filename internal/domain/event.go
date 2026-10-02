package domain

import "time"

// Event types emitted on the pipeline/job lifecycle (ADR 0001).
const (
	EventPipelineStarted  = "pipeline.started"
	EventPipelineFinished = "pipeline.finished"
	EventJobStarted       = "job.started"
	EventJobFinished      = "job.finished"
)

// Event is one lifecycle event envelope for the event bus (ADR 0001).
// The event's id is the Redis stream entry ID assigned by XADD; it is not
// part of the JSON body.
type Event struct {
	Type       string            `json:"type"`
	OccurredAt time.Time         `json:"occurred_at"`
	Pipeline   string            `json:"pipeline"`
	Job        string            `json:"job,omitempty"`
	Step       string            `json:"step,omitempty"`
	Payload    map[string]string `json:"payload,omitempty"`
}
