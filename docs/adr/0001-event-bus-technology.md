# ADR 0001: Event bus technology

- **Status:** Accepted (2026-09-18)
- **Decides:** gh-38 — Redis Streams vs Kafka/NATS for the results fan-out
- **Feeds:** gh-17 (MVP event-bus split)

## Context

The architecture (see README) splits result delivery into two sinks with different semantics:

- **Event bus** — ephemeral, multi-consumer, real-time. Consumers: notificationService, loggingService, dataVizService, testAnalysisService. Each consumer needs **every** event, with an independent cursor.
- **Results DB** — durable and queryable. Historical data lives here, not on the bus. Consumers that need history query the DB directly.

Constraints that follow from the product principles:

- **Scale:** a self-hosted CI platform. Event volume is pipeline/job/step lifecycle transitions — tens per run, runs in the single digits per minute. Nowhere near broker stress territory.
- **Ops appetite:** single-binary philosophy, docker-compose-able on one host. The moat is the intelligence layer; infra plumbing is deliberately minimized.
- **Language:** Go everywhere. Client maturity matters more than raw throughput.
- **Delivery:** at-least-once is required. A loggingService restart during a run must not lose events; pub/sub fire-and-forget is disqualified.

## Options considered

### Redis Pub/Sub — rejected

Fire-and-forget: no consumer groups, no persistence, messages lost while a consumer is disconnected or down. Fails the at-least-once requirement even for v1. Mentioned only to record why plain Redis Pub/Sub isn't the answer despite Redis itself winning.

### Redis Streams — ✅ chosen

- **Consumer groups** (`XREADGROUP`/`XACK`) give each service an independent cursor with at-least-once delivery — exactly the fan-out shape needed.
- **`XAUTOCLAIM`** recovers entries stuck with a crashed consumer; the main gotcha (a `0-0` cursor means end-of-pending, not "no pending") is a documented pattern, not a design flaw.
- **Retention via `MAXLEN`/`XTRIM`** bounds memory. The bus is allowed to be lossy over time precisely because the results DB owns history — this is a feature of the two-sink split, not a compromise.
- **Ops footprint:** a Redis (or Valkey) container the team would want anyway for caching/coordination; one more `docker compose` service, not a
  new operational discipline.
- **Go client:** `github.com/redis/go-redis/v9` is mature and ubiquitous.
- **Licensing:** Redis 8 offers AGPLv3 (May 2025); Valkey (Linux Foundation, BSD) is a drop-in alternative with identical stream semantics if a permissive license is preferred. No impact on this decision.

Trade-offs accepted: retention is memory-bound (bounded by `MAXLEN`, fine at this volume); a stream is single-sharded (no partition parallelism — irrelevant here); durability requires AOF enabled (one config line).

### NATS JetStream — runner-up

Genuinely good fit: single Go binary, at-least-once streams, subject-based routing with wildcards, strong Go client. Chosen against because it is a second system to run and monitor for capabilities v1 doesn't need: wildcard subject routing (four consumers can filter client-side at this volume) and multi-node clustering (single host today). **This is the migration path if the platform outgrows one box**, not the starting point.

### Kafka — rejected

Partitioned parallelism, unbounded disk-backed retention, and a large ecosystem — all pointless below thousands of events/second, in exchange for JVM + KRaft/ZooKeeper operational weight. Directly violates the "don't spend design effort on infra plumbing" principle for this product. Revisit only if the platform becomes a multi-tenant service.

## Decision

**Redis Streams** is the event bus for v1.

Operational shape (binding for gh-17 implementation):

1. **One stream** (`odyssey:events`), JSON envelope
   (`{id, type, occurred_at, pipeline, job, step, payload}`), emitted by the orchestrator on pipeline/job/step lifecycle transitions.
2. **One consumer group per service**, created `MKSTREAM` at consumer startup; consumers ack (`XACK`) only after successful handling.
3. **Stuck-entry recovery** via `XAUTOCLAIM` with a min-idle threshold (~5 minutes); claimed deliveries increment an attempt counter, poison messages dead-letter to a `odyssey:dead` stream after N attempts.
4. **Retention:** `MAXLEN ~ 100000` approximate trimming on `XADD`. The results DB is the system of record; nothing replays from the bus.
5. **Durability:** AOF enabled (`appendonly yes`) on the Redis instance.
6. **Graceful degradation:** if the bus is unreachable, pipeline execution proceeds unaffected and emission failures are logged, never fatal. The engine stays runnable without Redis (this also keeps the Docker test suites Redis-free).

No generic `Bus` abstraction hides multiple implementations — `internal/bus` is a thin Redis adapter, and the engine's seams are the small consumer-owned interfaces (`orchestrator.EventSink`, `runner.StepSink`). Per design principles, an abstraction that only re-wraps one implementation doesn't earn its weight; if a second bus ever arrives, extract a generic interface then. (Corrected 2026-10-09: the original wording predated the seam interfaces the code actually defines.)

## When to revisit

- Multi-host / clustered deployment → NATS JetStream.
- Sustained throughput in the thousands of events/second, or a need for wildcard topic routing → NATS JetStream.
- Multi-tenant SaaS with external data pipelines → re-evaluate Kafka.

## References

- README: architecture, event bus vs results DB split
- antirez, "Streams consumer patterns" — consumer-group recovery idioms
- Valkey docs: streams intro (`XAUTOCLAIM` cursor semantics)
- Redis blog: AGPLv3 licensing (Redis 8, May 2025)
