#!/system/bin/sh
#
# Nomad agent launcher for Android.
#
# This script is the supported way to start Nomad so that it survives the
# shell that launched it and can be restarted by an init hook.
#
# It exists because starting nomad directly from `adb shell` or from a
# foreground service has two problems on Android:
#
# 1. The process is killed when its parent session ends (SIGHUP).
# 2. os.Executable() may not point at an executable file, so the agent
# cannot re-execute itself for the task executor and logmon.
#
# Usage: nomad-android.sh [start|stop|restart|status]
#
# Configuration is read from $NOMAD_CONF (default /data/local/nomad/android.hcl).

set -u

LOG=/data/local/nomad/agent.log
PIDFILE=/data/local/nomad/agent.pid
CONF=${NOMAD_CONF:-/data/local/nomad/android.hcl}

# Resolve the Nomad binary. NOMAD_EXECUTABLE wins; otherwise probe the
# locations that are executable on Android, in the same order as the built-in
# fallback in helper/subproc.
resolve_bin() {
 if [ -n "${NOMAD_EXECUTABLE:-}" ] && [ -x "$NOMAD_EXECUTABLE" ]; then
 echo "$NOMAD_EXECUTABLE"
 return 0
 fi
 for candidate in \
 /system/bin/nomad \
 /system/xbin/nomad \
 /data/local/tmp/nomad \
 /data/local/nomad/nomad
 do
 if [ -x "$candidate" ]; then
 echo "$candidate"
 return 0
 fi
 done
 return 1
}

# alive reports whether $1 is a running process. Android always has /proc;
# desktop systems used for local testing may not, so fall back to kill -0.
alive() {
 if [ -d "/proc/$1" ]; then
 return 0
 fi
 kill -0 "$1" 2>/dev/null
}

# alive_any reports whether any pid in the whitespace-separated list is alive.
alive_any() {
 for P in $1; do
 if alive "$P"; then
 return 0
 fi
 done
 return 1
}

running_pid() {
 FOUND=""
 if [ -f "$PIDFILE" ]; then
 for P in $(cat "$PIDFILE"); do
 if alive "$P"; then
 FOUND="$FOUND $P"
 fi
 done
 fi
 echo $FOUND
 if [ -n "$FOUND" ]; then
 return 0
 fi
 return 1
}

# find_pid prints the pid of the first process whose executable basename
# matches $1. Android ships toybox pidof; desktop shells usually only have
# pgrep, so support both.
# find_pids prints every pid of a process whose command matches $1. Android
# ships toybox pidof; desktop shells usually only have pgrep, so support both.
find_pids() {
 name=$1
 if command -v pidof >/dev/null 2>&1; then
 pidof "$name" 2>/dev/null | tr ' ' '\n'
 else
 pgrep -f "/$name( |$)" 2>/dev/null
 fi
}

# find_pid prints the first matching pid, for start/status.
find_pid() {
 find_pids "$1" | head -n1
}

start_agent() {
 if P=$(running_pid); then
 echo "already running (pid $P)"
 return 0
 fi

 BIN=$(resolve_bin) || {
 echo "no executable nomad binary found; set NOMAD_EXECUTABLE" >&2
 return 1
 }

 if [ ! -f "$CONF" ]; then
 echo "config not found: $CONF" >&2
 return 1
 fi

 mkdir -p /data/local/nomad
 # The pid of setsid is NOT the pid of the agent: setsid forks and exits.
 # Recording "$!" would write a pid that is already gone, which makes status
 # report "not running" and stop a no-op. Discover the real pid instead.
 #
 # setsid detaches from the controlling terminal so the agent is reparented
 # to init instead of dying with the shell that started it. It is absent on
 # some desktop systems, where nohup is a close enough substitute for a
 # local test run.
 if command -v setsid >/dev/null 2>&1; then
 	setsid "$BIN" agent -config="$CONF" >>"$LOG" 2>&1 </dev/null &
 else
 	nohup "$BIN" agent -config="$CONF" >>"$LOG" 2>&1 </dev/null &
 fi

 # Find the agent pid. setsid forks and exits, so "$!" is not the agent.
 BASE=$(basename "$BIN")
 P=""
 i=0
 while [ "$i" -lt 20 ]; do
 P=$(find_pid "$BASE")
 if [ -n "$P" ]; then
 break
 fi
 sleep 1
 i=$((i +1))
 done

 if [ -z "$P" ]; then
 echo "agent did not start; see $LOG" >&2
 return 1
 fi

	# Record every pid started from this binary: the agent plus whichever
	# helpers (executor) are already running, so stop can signal the group
	# even when the agent dies first.
	ALL=$(find_pids "$BASE")
	echo $ALL > "$PIDFILE"
	echo "started pid $(echo $ALL | tr ' ' ',') using $BIN"
}

stop_agent() {
 TARGETS=$(running_pid)
 if [ -z "$TARGETS" ]; then
 echo "not running"
 rm -f "$PIDFILE"
 return 0
 fi
 # Stop every pid in the recorded set; a crashed agent can leave the executor
 # behind, and both must be signalled for the mount cleanup to succeed.
 for P in $TARGETS; do
 kill -TERM "$P" 2>/dev/null
 done
 i=0
 while [ "$i" -lt 30 ]; do
 if ! alive_any "$TARGETS"; then
 break
 fi
 sleep 1
 i=$((i +1))
 done
 for P in $TARGETS; do
 if alive "$P"; then
 echo "pid $P still running after 30s, sending KILL"
 kill -KILL "$P" 2>/dev/null
 fi
 done
 rm -f "$PIDFILE"
 echo "stopped"
}

case "${1:-start}" in
 start) start_agent ;;
 stop) stop_agent ;;
 restart)
 stop_agent
 start_agent
 ;;
 status)
 if P=$(running_pid); then
 echo "running (pid $P)"
 else
 echo "not running"
 fi
 ;;
 *)
 echo "usage: $0 {start|stop|restart|status}" >&2
 exit 2
 ;;
esac
