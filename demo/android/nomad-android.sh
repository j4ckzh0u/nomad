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

running_pid() {
 if [ -f "$PIDFILE" ]; then
 P=$(cat "$PIDFILE")
 if [ -n "$P" ] && [ -d "/proc/$P" ]; then
 echo "$P"
 return 0
 fi
 fi
 return 1
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
 # setsid detaches from the controlling terminal so the agent is reparented
 # to init instead of dying with the shell that started it.
 setsid "$BIN" agent -config="$CONF" >>"$LOG" 2>&1 </dev/null &
 echo $! > "$PIDFILE"
 echo "started pid $(cat "$PIDFILE") using $BIN"
}

stop_agent() {
 if ! P=$(running_pid); then
 echo "not running"
 return 0
 fi
 # SIGTERM lets the client unmount alloc dirs and shut down cleanly.
 kill -TERM "$P" 2>/dev/null
 i=0
 while [ -d "/proc/$P" ] && [ "$i" -lt 30 ]; do
 sleep 1
 i=$((i +1))
 done
 if [ -d "/proc/$P" ]; then
 echo "still running after 30s, sending KILL"
 kill -KILL "$P" 2>/dev/null
 fi
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
