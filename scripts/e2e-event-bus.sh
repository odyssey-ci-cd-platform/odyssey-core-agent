#!/usr/bin/env bash
# End-to-end check: a hosted gRPC run emits lifecycle events onto a live Redis.
#
# Spawns its own Redis (docker), starts the real server with
# ODYSSEY_REDIS_ADDR pointed at it, triggers one pipeline run through the
# real client, then asserts the odyssey:events stream contents. Cleans up
# everything it created. Needs: docker, go. Exit 0 = pass.
set -euo pipefail
cd "$(dirname "$0")/.."

SRV_PORT=50995
REDIS_PORT=6399
REDIS_CONTAINER=odyssey-e2e-redis
SRV_BIN=./.e2e-od-server
CLI_BIN=./.e2e-od-client
SRV_LOG=.e2e-server.log
CLI_OUT=.e2e-client.out
PROJECT=.e2e-project

cleanup() {
	[[ -n "${SRV_PID:-}" ]] && kill "$SRV_PID" 2>/dev/null || true
	docker rm -f "$REDIS_CONTAINER" >/dev/null 2>&1 || true
	rm -rf "$SRV_BIN" "$CLI_BIN" "$SRV_LOG" "$CLI_OUT" "$PROJECT"
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
ODYSSEY_ADDR=$SRV_PORT ODYSSEY_REDIS_ADDR=localhost:$REDIS_PORT "$SRV_BIN" >"$SRV_LOG" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 25); do
	grep -q "event bus enabled" "$SRV_LOG" && break
	sleep 0.2
done
grep -q "event bus enabled" "$SRV_LOG" || fail "server did not log event bus enabled"

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

echo "PASS: 6 lifecycle events for e2e-smoke on odyssey:events"
echo "$STREAM" | grep -o '"type":"[^"]*"'
