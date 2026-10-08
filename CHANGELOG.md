# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Format

- Latest changes go at the top, under `[Unreleased]`, until a version is tagged.
- Group every entry under one of these headings, in this order:

  - **Added** — new features.
  - **Changed** — behavior changes to existing features.
  - **Deprecated** — soon-to-be-removed features.
  - **Removed** — removed features.
  - **Fixed** — bug fixes.
  - **Security** — vulnerability fixes.

- One entry per change: `- <summary>.` — sentence case, past tense.
- Reference the issue when one exists: `- <summary> (gh-<N>).`
- Only headings with entries appear in a release. Empty headings are omitted.
- Commits and branches keep the `gh-<N>` convention; entries summarize, not enumerate commits.

## [Unreleased]

### Added
- Lifecycle events carry a run ID and finished events survive run cancellation (gh-67).
- Repository audit process (`audit/PROCESS.md`) and the first recorded audit, evaluated at 7d7be54 (gh-60).
- gRPC contract: step results carry stdout, stderr, error text, and duration separately; job results carry error and duration; stage, pipeline, and response carry durations (gh-66).
- Results DB technology decision record: embedded SQLite chosen for v1, normalized schema, single results-recorder writer, direct read access for analytics services (gh-17).
- Fan-out consumer skeleton: `internal/consumer` with consumer groups, at-least-once delivery, claim recovery, and dead-lettering to `odyssey:dead`, plus an `events-logger` reference consumer (gh-17).
- Step started/finished events emitted by the runner during job execution, payload carrying status only (gh-52).
- End-to-end event bus smoke script (`scripts/e2e-event-bus.sh`): live Redis in docker, real server and client, asserts the stream contents; documented as RUNBOOK §1.5.
- Hosted gRPC runs emit lifecycle events to the event bus when `ODYSSEY_REDIS_ADDR` is set; without it the server runs with the bus disabled (gh-52).
- Pipeline and job lifecycle events published to the Redis Streams event bus, with emission failures logged and never fatal (gh-52).
- Event bus technology decision record: Redis Streams chosen for v1 (gh-38).
- Process scaffolding ported from the cubicle project: issue-first workflow with solution-template and PR-link automation, one-shot ship/merge scripts, RUNBOOK.md with recorded baseline, gitignored STATE.md, and a `make check` gate.
- CI workflow running `make check` on every PR and push to main; branch protection on main requires the `check` status and pull requests.

### Changed
- Pipeline TOML files with unknown keys are rejected at load, naming the file and keys; the undocumented job `name` key is gone from the example and fixtures (gh-68).
- A declared stage with no jobs now fails validation instead of aggregating to Pending forever (gh-69).
- Jobs within a stage are ordered deterministically by job key (gh-70).
- `make check` enforces gofmt cleanliness and module tidiness alongside vet and tests (gh-78).

### Fixed
- Step results carry the real process exit code; non-zero exits are no longer reported as infrastructure errors, and infrastructure faults mid-step surface as `StatusErrored` (gh-65).
