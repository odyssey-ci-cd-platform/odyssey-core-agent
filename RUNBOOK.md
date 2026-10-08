# Runbook

Step-by-step procedures for routine operations, maintenance tasks, and incident resolution on Odyssey.

Every procedure ends with a verification step; if verification fails, go to the matching incident in §3.

Update a procedure in the same commit that changes what it does.

## 1. Routine operations

### 1.1 Build and test

1. `make check`

Verify: everything green — that is the recorded baseline (Incident 3.1).

### 1.2 Regenerate gRPC code

1. Edit `api/v1/odyssey.proto`.
2. `make proto` — needs `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc` on `PATH`.
3. `make check`

Verify: build passes with the regenerated `gen/proto/v1` code. Never hand-edit `gen/`.

### 1.3 Ship a work item

Rules in `AGENTS.md` → Workflow. Sequence:

1. Multi-commit work → GitHub issue first (states the outcome, never the code change); a workflow posts the solution template as the first comment automatically.
   First actionable: fill it in by editing the posted comment — every header kept, N/A where not required.
   One-commit task → skip this step.
2. `git checkout -b <branch>` — `feature/<issue-number>`, or issueless `type/slug`.
   All changes land through PRs: main is protected (required `check` status, no direct pushes, admins included).
3. Write the failing intent test first — red before the change.
4. Change until `make check` matches the recorded baseline or better.
5. Commit: `gh-<issue_number>: <summary>`, imperative mood.
6. `git push -u origin <branch>` and open a PR — the `PR: <url>` comment on the issue posts automatically.
   One shot: `scripts/ship-pr.sh <branch> <title> [body-file]`.
7. Watch CI finish on the PR head — `gh pr checks <branch> --watch`; the required `check` must be green before merging.
8. Merge the PR — one shot: `scripts/merge-pr.sh <branch>`.
9. `git checkout main && git pull && git branch -d <branch>`.

Verify: PR merged, branch gone on both ends. `make check` on main matches the recorded baseline.

### 1.4 Docs changes

1. Run `.agents/bin/check-docs.sh` before committing modified Markdown.

Verify: no mid-sentence hard-wrap findings in changed files.

### 1.5 Event bus end-to-end smoke

1. Run `scripts/e2e-event-bus.sh`.

The script spawns its own Redis (docker, `redis:7-alpine`), starts the server with `ODYSSEY_REDIS_ADDR` pointed at it, triggers one pipeline run through the real gRPC client, and asserts the `odyssey:events` stream holds exactly the four lifecycle events tagged with the fixture pipeline.

Verify: `PASS: 4 lifecycle events for e2e-smoke on odyssey:events` and exit code 0; the script cleans up its container, binaries, and fixture on both pass and fail.

Use it when changing the bus, orchestrator emission, or server wiring — `make check` covers the seams with miniredis, not a live Redis.

## 2. Maintenance tasks

### 2.1 Record a decision or status change

1. Edit `README.md` (design decisions, open questions) or add an ADR under `docs/adr/` — not this file.
2. Add a `CHANGELOG.md` entry under `[Unreleased]`.

Verify: `grep -n` finds the new line in both files.

## 3. Incident resolution

Format: symptom → diagnose → resolve.

### 3.1 Suite state vs the recorded baseline

- Symptom: everything green — that is the recorded baseline; no action.
- Symptom: failures or hangs confined to `internal/runner/docker_test.go` — run `docker info` first; a stopped daemon fails those tests for infra reasons, not code reasons.
- Symptom: any other failure — regression; fix before commit.

### 3.2 `make proto` fails

- Symptom: `protoc` not found, or a `protoc-gen-*` plugin missing.
- Diagnose: the error names the missing binary.
- Resolve: install the missing tool on `PATH`; never hand-edit `gen/`.

### 3.3 `gh pr merge` refuses with `Required status check "check" is expected`

- Symptom: merging immediately after a push fails; the merge command and CI raced.
- Diagnose: the required `check` on the head commit had not finished when the merge was attempted.
- Resolve: `gh pr checks <branch> --watch` first, then re-run `scripts/merge-pr.sh <branch>`.

### 3.4 New incident

Any failure that costs more than a minute to diagnose gets a section here: symptom → diagnose → resolve, three bullets each.
