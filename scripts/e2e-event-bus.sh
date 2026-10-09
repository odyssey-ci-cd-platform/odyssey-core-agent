#!/usr/bin/env bash
# End-to-end check: a hosted gRPC run emits lifecycle events onto a live Redis
# and lands durably in the SQLite results DB.
#
# Spawns its own Redis (docker), starts the real server with
# ODYSSEY_REDIS_ADDR pointed at it, triggers one pipeline run through the
# real client, then asserts the odyssey:events stream contents and the
# recorded runs/jobs/steps rows. Cleans up everything it created. Needs:
# docker, go, python3. Exit 0 = pass.
set -euo pipefail
cd "$(dirname "$0")/.."

SRV_PORT=50995
REDIS_PORT=6399
REDIS_CONTAINER=odyssey-e2e-redis
SRV_BIN=./.e2e-od-server
CLI_BIN=./.e2e-od-client
LOG_BIN=./.e2e-events-logger
SRV_LOG=.e2e-server.log
CLI_OUT=.e2e-client.out
LOG_OUT=.e2e-consumer.log
RESULTS_DB=.e2e-results.db
PROJECT=.e2e-project

cleanup() {
	[[ -n "${SRV_PID:-}" ]] && kill "$SRV_PID" 2>/dev/null || true
	[[ -n "${LOG_PID:-}" ]] && kill "$LOG_PID" 2>/dev/null || true
	docker rm -f "$REDIS_CONTAINER" >/dev/null 2>&1 || true
	rm -rf "$SRV_BIN" "$CLI_BIN" "$LOG_BIN" "$SRV_LOG" "$CLI_OUT" "$LOG_OUT" "$RESULTS_DB" "$RESULTS_DB-wal" "$RESULTS_DB-shm" "$PROJECT"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

# 1. Live Redis in docker.
docker run -d --rm --name "$REDIS_CONTAINER" -p "${REDIS_PORT}:6379" redis:7-alpine >/dev/null
for _ in $(seq 1 20); do
	docker exec "$REDIS_CONTAINER" redis-cli PING 2>/dev/null | grep -q PONG && break
	sleep 0.5
done
docker exec "$REDIS_CONTAINER" redis-cli PING | grep -q PONG || fail "redis did not come up"

# 2. Fixture project: one job, one echo step.
mkdir -p "$PROJECT/.odyssey"
cat > "$PROJECT/.odyssey/pipeline.toml" <<'EOF'
[pipeline]
name = "e2e-smoke"
stages = ["stage"]

[jobs.echo-job]
stage = "stage"
image = "alpine"
steps = [{ name = "hello", run = "echo hi" }]
EOF

# 3. Build and start the server under test, wired to the live Redis.
go build -o "$SRV_BIN" ./cmd/server
go build -o "$CLI_BIN" ./cmd/client
go build -o "$LOG_BIN" ./cmd/events-logger
# gh-74 made ODYSSEY_ADDR verbatim; a bare port no longer parses, and a
# colon-prefixed one would bind every interface — bind localhost explicitly.
ODYSSEY_ADDR=localhost:$SRV_PORT ODYSSEY_REDIS_ADDR=localhost:$REDIS_PORT ODYSSEY_RESULTS_DB=$RESULTS_DB "$SRV_BIN" >"$SRV_LOG" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 25); do
	grep -q "event bus enabled" "$SRV_LOG" && break
	sleep 0.2
done
grep -q "event bus enabled" "$SRV_LOG" || fail "server did not log event bus enabled"

# 3b. A live subscriber joins before the run: fan-out while producing.
"$LOG_BIN" -redis localhost:$REDIS_PORT -group e2e-logger >"$LOG_OUT" 2>&1 &
LOG_PID=$!

# 4. Trigger one real run through the gRPC client (executes an alpine container).
"$CLI_BIN" -addr localhost:$SRV_PORT -timeout 120s "$PROJECT" >"$CLI_OUT" 2>&1 \
	|| fail "client run failed (see $CLI_OUT)"
grep -q "Status: STATUS_PASSED" "$CLI_OUT" || fail "pipeline did not pass (see $CLI_OUT)"

# 5. Assert the stream: exactly the six lifecycle events, tagged with the pipeline.
XLEN=$(docker exec "$REDIS_CONTAINER" redis-cli XLEN odyssey:events)
[[ "$XLEN" == "6" ]] || fail "XLEN odyssey:events = $XLEN, want 6"

STREAM=$(docker exec "$REDIS_CONTAINER" redis-cli --raw XRANGE odyssey:events - +)
for type in pipeline.started job.started step.started step.finished job.finished pipeline.finished; do
	echo "$STREAM" | grep -q "\"type\":\"$type\"" || fail "missing event $type"
done
NAMES=$(echo "$STREAM" | grep -o '"pipeline":"[^"]*"' | sort -u)
[[ "$NAMES" == '"pipeline":"e2e-smoke"' ]] || fail "unexpected pipeline names: $NAMES"

# 6. The live subscriber handled all six events and acked them.
deadline=$((SECONDS + 10))
until [[ $(grep -c "^type=" "$LOG_OUT" 2>/dev/null || echo 0) == "6" ]]; do
	[[ $SECONDS -lt $deadline ]] || fail "consumer did not handle 6 events (see $LOG_OUT)"
	sleep 0.2
done
PENDING=$(docker exec "$REDIS_CONTAINER" redis-cli --raw XPENDING odyssey:events e2e-logger | head -1)
[[ "$PENDING" == "0" ]] || fail "consumer group pending = $PENDING, want 0 (acked)"

# 7. The recorder landed the run, job, and step durably (ADR 0002).
check_db() {
python3 - "$RESULTS_DB" <<'PYEOF'
import sqlite3, sys

db = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
runs = db.execute("SELECT run_id, pipeline, status, started_at, finished_at FROM runs").fetchall()
assert len(runs) == 1, f"runs rows = {runs}, want 1"
run = runs[0]
assert run[1] == "e2e-smoke", f"pipeline = {run[1]}"
assert run[2] == "passed", f"status = {run[2]}"
assert run[3] and run[4], f"timestamps missing: {run}"
jobs = db.execute("SELECT name, stage, status FROM jobs").fetchall()
assert jobs == [("echo-job", "stage", "passed")], f"jobs = {jobs}"
steps = db.execute("SELECT name, status, exit_code, duration_ms, payload FROM steps").fetchall()
assert len(steps) == 1, f"steps rows = {steps}, want 1"
step = steps[0]
assert step[0] == "hello" and step[1] == "passed", f"step = {step}"
assert step[2] == 0, f"exit_code = {step[2]}"
assert step[3] is not None and step[3] >= 0, f"duration_ms = {step[3]}"
assert step[4] == "{}", f"payload = {step[4]}"
print(f"rows ok: run {run[0]} passed, job echo-job, step hello exit 0 in {step[3]}ms")
PYEOF
}
deadline=$((SECONDS + 10))
until check_db >/dev/null 2>&1; do
	[[ $SECONDS -lt $deadline ]] || { check_db || fail "results database did not receive the run"; }
	sleep 0.2
done

echo "PASS: 6 lifecycle events for e2e-smoke on odyssey:events; live subscriber handled and acked all 6; run recorded in $RESULTS_DB"
echo "$STREAM" | grep -o '"type":"[^"]*"'
