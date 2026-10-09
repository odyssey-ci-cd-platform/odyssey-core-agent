# ADR 0002: Results DB technology

- **Status:** Accepted (2026-10-08)
- **Decides:** gh-17 — where the durable half of the event-bus/results-DB split lives
- **Feeds:** gh-18 (flakiness detection), gh-42 (loggingService), gh-43 (dataVizService)

## Context

The architecture splits result delivery into two sinks with different semantics (ADR 0001): the event bus is ephemeral and real-time, and the results DB is durable and queryable. ADR 0001 deliberately left the DB side open; this record closes it.

Constraints that follow from the product principles and what now exists:

- **Scale:** pipeline/job/step lifecycle events, tens per run, runs in the single digits per minute. The results DB at v1 holds thousands of rows, not millions. Every candidate is over-provisioned on throughput; the decision is ops weight and growth path, not performance.
- **Read shape:** the moat is the intelligence layer — flakiness detection (gh-18), trend analytics, test ownership. Those are SQL-shaped questions over per-run/per-job/per-step history: "all outcomes of step X across the last N runs of pipeline P".
- **Write shape:** exactly one writer is realistic — a `results-recorder` consumer on the bus (ADR 0001: the DB is the system of record; the bus is lossy and never replayed into it). Writes are small, batched, and single-threaded per pipeline.
- **Deployment:** single binary, docker-compose-able on one host; infra plumbing is deliberately minimized. A second always-on database server is a real cost, not a checkbox.
- **Existing precedent:** the closest analog — Woodpecker CI (Go, self-hosted, single-host CI) — ships embedded SQLite as its default engine with Postgres/MySQL as opt-ins, with documented lock-contention failures only under concurrent write bursts. Our recorder is a single writer, which is exactly the pattern SQLite handles best.

## Options considered

### Embedded SQLite — ✅ chosen

- **Zero ops:** no server process, no users, no connection pooling, no `CREATE DATABASE`. The database is one file on a volume; the whole engine stays a single binary (pure-Go driver keeps it CGO-free and cross-compilable).
- **Read performance at our shape:** local file reads with no network round-trip; at thousands of rows, analytics queries are sub-millisecond. Irrelevant how either store benchmarks against the other at this volume — both are instant.
- **Durability story is known and bounded:** WAL mode gives concurrent readers with a single writer; Litestream (a config-only sidecar, not code) continuously ships WAL frames to S3-compatible storage with a ~1 second loss window. v1 ships WAL on a host volume; Litestream is documented in compose, adopted when a hosted deployment exists.
- **Growth path is honest:** SQLite's limits (one writer, one host) are exactly the triggers listed under "When to revisit". Reaching for them is a migration, not a rewrite — the recorder consumer is the only writer, so swapping the store behind it touches one package.
- **Trade-offs accepted:** single-host only (no multi-engine reads); lock contention is possible if something else writes to the file (the recorder-must-be-the-only-writer rule exists to prevent this); Litestream's ~1s window means a catastrophic host loss can lose the last second of results — acceptable, the bus has the same loss envelope and the runs themselves are reproducible.

### PostgreSQL — deferred, the documented growth path

- The standard choice with the best tooling, and the right answer the day results must be read from multiple hosts or the platform becomes multi-tenant (gh-28).
- Rejected **for v1** on ops weight: a second stateful service to run, monitor, back up, and version-upgrade on every host the engine installs on — for a workload that cannot tell the difference. "Postgres, because eventually" is the ops-plumbing spend the product principles defer.
- The `database/sql` access layer keeps the door open: the recorder and readers are written against standard SQL that both stores speak.

### ClickHouse / TimescaleDB / DuckDB — rejected

- Column stores and OLAP engines earn their keep at millions of rows and complex aggregations. At this volume they are a second database to operate for queries SQLite answers instantly. Revisit only if trend analytics outgrow SQL-shaped rollups (not on any roadmap).

### Append-only event table + views — rejected for the core schema

- Trivially flexible, but it pushes every analytics question into view SQL over raw envelopes, and schema evolution becomes view rewrites.
- Chosen direction instead: normalized `runs`/`jobs`/`steps` tables (see Decision), with a JSON `payload` column on `steps` reserved for future sub-step data (e.g. per-test outcomes for gh-18) so the intelligence layer is never blocked on a migration.

## Decision

**SQLite, embedded, pure-Go driver** is the results DB for v1.

Operational shape (binding for the recorder slice):

1. **One writer:** a `results-recorder` consumer (ADR 0001 group semantics) is the only process that writes the DB. All other components — viz, analytics, UI — are readers. This rule is what makes the single-writer constraint a non-issue.
2. **Driver:** `modernc.org/sqlite` (pure Go, no CGO) behind `database/sql`; WAL journal mode enabled at open. CGO-free keeps the engine a single static binary and cross-compilation trivial.
3. **Schema, normalized:** `runs` (pipeline name, status, started/finished timestamps, unique on `(pipeline, started_at)`), `jobs` (run id, name, stage, status), `steps` (job id, name, status, exit code, duration) — mirroring the event envelope's three levels plus a JSON `payload` column on `steps` for future test-level outcomes. Migrations are versioned SQL files applied by goose (library mode, `embed.FS`), so the binary migrates its own database on startup.
4. **Run identity:** runs key on the envelope's run id (added by gh-67, superseding this record's original `(pipeline, started_at)` keying — the amendment anticipated here). The recorder attaches job/step events in stream order and rejects events whose parent run is unknown — at-least-once redelivery stays idempotent via the unique indexes, and missing identity dead-letters loudly instead of recording indistinguishable rows.
5. **Access pattern (settles the README open question):** `dataVizService` and the analytics services query the results DB **directly on day one** for history; the bus stays their real-time channel. This is the point of the two-sink split: history questions go to the DB, live questions go to the bus, and neither service waits on the other.
6. **Retention:** none. The results DB is the system of record; rows accumulate (Woodpecker made the same call — archival out of scope). Disk is the binding constraint and is a deploy-time concern.
7. **Durability:** WAL on a host volume in v1; Litestream to object storage documented in the compose file and enabled for hosted deployments (gh-28 territory).

No `DB` interface abstraction is introduced — the same rule as ADR 0001's bus: the recorder owns its SQL; if a second store ever arrives, extract the interface then.

## When to revisit

- Multi-host reads or a second engine instance needing the same DB → Postgres (the recorder is the only writer, so the migration is contained).
- Multi-tenant / managed offering (gh-28) → Postgres with per-tenant policy.
- Sustained `database is locked` pressure → first check for a rogue writer; if genuinely write-bound, Postgres.

## References

- ADR 0001: event bus technology, two-sink split, graceful degradation
- README: architecture (event bus vs results DB), open questions (dataVizService access — settled here)
- Woodpecker CI docs: SQLite default engine, archival stance; woodpecker-ci/woodpecker#4368 (lock contention under concurrent writers)
- Litestream docs: alternatives and durability windows
- SQLite Drivers Benchmarks Game (Feb 2026): modernc.org/sqlite vs mattn/go-sqlite3 vs ncruces
