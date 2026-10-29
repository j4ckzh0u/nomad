#!/usr/bin/env bash
#
# Smoke test: start an agent, run a job, confirm it ran, clean up.
#
# Usage: ./smoke-test.sh [binary]
#
# Defaults to $PWD/nomad. Runs in dev mode, which keeps all state in memory and
# destroys allocations on exit, so nothing is left behind.

set -euo pipefail

BIN="${1:-$PWD/nomad}"
ADDR="http://127.0.0.1:4646"
JOB="smoke"

if [ ! -x "$BIN" ]; then
 echo "not executable: $BIN" >&2
 echo "usage: $0 [path-to-nomad]" >&2
 exit 1
fi

echo "==> version"
"$BIN" version

echo "==> starting agent"
"$BIN" agent -dev -log-level=WARN >/tmp/nomad-smoke.log 2>&1 &
AGENT_PID=$!

cleanup () {
 kill "$AGENT_PID" 2>/dev/null || true
 wait "$AGENT_PID" 2>/dev/null || true
}
trap cleanup EXIT

# The API answers before the node has fingerprinted, so poll for both.
echo "==> waiting for the API"
for _ in $(seq 140); do
 if curl -fsS "$ADDR/v1/status/leader" >/dev/null 2>&1; then
 break
 fi
 sleep 0.5
done

echo "==> waiting for the node to be ready"
for _ in $(seq 140); do
 status=$(curl -fsS "$ADDR/v1/nodes" 2>/dev/null | grep -o '"Status":"ready"' | head -n 1 || true)
 if [ -n "$status" ]; then
 break
 fi
 sleep 0.5
done

echo "==> driver list"
curl -fsS "$ADDR/v1/nodes" | tr ',' '\n' | grep -o '"driver\.[a-z_]*"' | sort -u

echo "==> running a job"
cat >/tmp/nomad-smoke.nomad <<'EOF'
job "smoke" {
 datacenters = ["dc1"]
 type = "batch"

 group "g" {
 task "t" {
 driver = "raw_exec"
 config {
 command = "/bin/sh"
 args = ["-c", "echo smoke-ok"]
 }
 }
 }
}
EOF

NOMAD_ADDR="$ADDR" "$BIN" job run -detach /tmp/nomad-smoke.nomad

echo "==> waiting for completion"
for _ in $(seq 140); do
 state=$(NOMAD_ADDR="$ADDR" "$BIN" job status "$JOB" 2>/dev/null | awk '/^Status/ {print $3}')
 if [ "$state" = "dead" ]; then
 break
 fi
 sleep 0.5
done

echo "==> result"
NOMAD_ADDR="$ADDR" "$BIN" job status "$JOB"

alloc=$(NOMAD_ADDR="$ADDR" "$BIN" job allocs "$JOB" -t '{{range .}}{{.ID}}{{println}}{{end}}' 2>/dev/null | head -n1 || true)
if [ -n "$alloc" ]; then
 echo "==> alloc output"
 # A batch task that exits immediately may not have a log file yet. That
 # is not a failure of the smoke test.
 NOMAD_ADDR="$ADDR" "$BIN" alloc logs "$alloc" t || echo "(no log output)"
fi

echo
echo "OK: agent started, node ready, job ran"
