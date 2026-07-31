package server

import (
	"fmt"

	odysseyv1 "bitbucket.org/odyssey-ci/odyssey-core-agent/gen/proto/v1"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
)

// domainStatusToProto maps a domain.Status slug to the corresponding
// protobuf Status enum. Unknown or empty slugs produce STATUS_UNSPECIFIED.
func domainStatusToProto(s domain.Status) odysseyv1.Status {
    switch s.String() {
    case "passed":
        return odysseyv1.Status_STATUS_PASSED
    case "failed":
        return odysseyv1.Status_STATUS_FAILED
    case "errored":
        return odysseyv1.Status_STATUS_ERRORED
    case "pending":
        return odysseyv1.Status_STATUS_PENDING
    case "running":
        return odysseyv1.Status_STATUS_RUNNING
    case "skipped":
        return odysseyv1.Status_STATUS_SKIPPED
    default:
        return odysseyv1.Status_STATUS_UNSPECIFIED
    }
}

// domainStepResultToProto converts a single domain step result to its
// protobuf form. Stdout and stderr are combined into the Output field.
func domainStepResultToProto(r domain.StepResult) *odysseyv1.StepResult {
	output := r.Stdout
	if r.Stderr != "" {
		output = fmt.Sprintf("%s\n%s", output, r.Stderr)
	}
	return &odysseyv1.StepResult{
		StepName: r.StepName,
		Output: output,
		ExitCode: int32(r.ExitCode),
		Status: domainStatusToProto(r.Status()),
	}
}

// domainJobResultToProto converts a single domain job result to its
// protobuf form. If SetupErr is set on the domain result, Status will be
// STATUS_ERRORED and StepResults will be empty.
func domainJobResultToProto(r domain.JobResult) *odysseyv1.JobResult {
	protoStepResults := make([]*odysseyv1.StepResult, 0, len(r.StepResults))
	for _, sr := range r.StepResults {
		protoStepResults = append(protoStepResults, domainStepResultToProto(sr))
	}
	return &odysseyv1.JobResult{
		JobName: r.JobName,
		Status: domainStatusToProto(r.Status()),
		StepResults: protoStepResults,
	}
}

// domainStageResultToProto converts a single domain stage result to its
// protobuf form, including all nested job results.
func domainStageResultToProto(r domain.StageResult) *odysseyv1.StageResult {
	protoJobResults := make([]*odysseyv1.JobResult, 0, len(r.JobResults))
	for _, jr := range r.JobResults {
		protoJobResults = append(protoJobResults, domainJobResultToProto(jr))
	}
	return &odysseyv1.StageResult {
		StageName: r.StageName,
		Status: domainStatusToProto(r.Status()),
		JobResults: protoJobResults,
	}
}

// domainPipelineResultToProto converts a complete domain pipeline result
// (including all nested stages, jobs, and steps) into the protobuf
// RunPipelineResponse returned to the caller.
func domainPipelineResultToProto(r domain.PipelineResult) odysseyv1.RunPipelineResponse {
	protoStageResults := make([]*odysseyv1.StageResult, 0, len(r.StageResults))
	for _, sr := range r.StageResults {
		protoStageResults = append(protoStageResults, domainStageResultToProto(sr))
	}
	return odysseyv1.RunPipelineResponse {
		PipelineName: r.PipelineName,
		Status: domainStatusToProto(r.Status()),
		StageResults: protoStageResults,
	}
}

