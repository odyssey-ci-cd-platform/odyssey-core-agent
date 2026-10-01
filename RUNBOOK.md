# Runbook

Step-by-step procedures for routine operations, maintenance tasks, and
incident resolution on Odyssey. Every procedure ends with a verification
step; if verification fails, go to the matching incident in §3. Update a
procedure in the same commit that changes what it does.

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

1. Multi-commit work → GitHub issue first (states the outcome, never the
   code change); a workflow posts the solution template as the first
   comment automatically. First actionable: fill it in by editing the
   posted comment — every header kept, N/A where not required. One-commit
   task → skip this step.
2. `git checkout -b feature/<issue-number>` (issueless housekeeping lands
   directly on main, per existing precedent).
3. Write the failing intent test first — red before the change.
4. Change until `make check` matches the recorded baseline or better.
5. Commit: `gh-<issue_number>: <summary>`, imperative mood.
6. `git push -u origin <branch>` and open a PR — the `PR: <url>` comment
   on the issue posts automatically. One shot: `scripts/ship-pr.sh
   <branch> <title> [body-file]`.
7. CI green → CHANGELOG.md entry under `[Unreleased]`, commit, push;
   CI runs again.
8. Merge the PR once the latest commit is green — one shot:
   `scripts/merge-pr.sh <branch>`.
9. `git checkout main && git pull && git branch -d <branch>`.

Verify: PR merged, branch gone on both ends, `make check` on main matches
the recorded baseline.

### 1.4 Docs changes

1. Run `.agents/bin/check-docs.sh` before committing modified Markdown.

Verify: no mid-sentence hard-wrap findings in changed files.

## 2. Maintenance tasks

### 2.1 Record a decision or status change

1. Edit `README.md` (design decisions, open questions) or add an ADR under
   `docs/adr/` — not this file.
2. Add a `CHANGELOG.md` entry under `[Unreleased]`.

Verify: `grep -n` finds the new line in both files.

## 3. Incident resolution

Format: symptom → diagnose → resolve.

### 3.1 Suite state vs the recorded baseline

- Symptom: everything green — that is the recorded baseline; no action.
- Symptom: failures or hangs confined to `internal/runner/docker_test.go`
  — run `docker info` first; a stopped daemon fails those tests for infra
  reasons, not code reasons.
- Symptom: any other failure — regression; fix before commit.

### 3.2 `make proto` fails

- Symptom: `protoc` not found, or a `protoc-gen-*` plugin missing.
- Diagnose: the error names the missing binary.
- Resolve: install the missing tool on `PATH`; never hand-edit `gen/`.

### 3.3 New incident

Any failure that costs more than a minute to diagnose gets a section
here: symptom → diagnose → resolve, three bullets each.
