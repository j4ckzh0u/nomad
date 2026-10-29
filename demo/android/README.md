# Nomad on Android

Configuration, launcher and example jobs for running the Nomad client on an
Android device with the `raw_exec` driver. Background and the list of code
changes are in `contributing/android.md`.

## Files

| File | Purpose |
| --- | --- |
| `android.hcl` | Single-node agent config: server + client, `raw_exec` only |
| `example.nomad` | One long-running `raw_exec` task |
| `stack.nomad` | Two instances with a rolling update, host volume, restart policy |
| `nomad-android.sh` | `start` / `stop` / `restart` / `status` launcher |
| `nomad.rc` | Android init service definition for boot persistence |
| `DEPLOYMENT.md` | The three deployment shapes and which one you are in |

## Quick start (rooted board)

```sh
adb push pkg/android_arm64/nomad.so /data/local/tmp/nomad
adb shell chmod 755 /data/local/tmp/nomad
adb shell mkdir -p /data/local/nomad /data/local/services
adb push android.hcl /data/local/nomad/android.hcl
adb push nomad-android.sh /data/local/nomad/
adb shell sh /data/local/nomad/nomad-android.sh start
adb forward tcp:4646 tcp:4646
nomad job run example.nomad
```

For an enforcing device without root, read `DEPLOYMENT.md` first: the binary
has to be packaged into the APK and `executable_path` (or `NOMAD_EXECUTABLE`)
has to be set.

## Stopping

`nomad-android.sh stop` deregisters every job through the agent API and then
signals the agent. Both steps are required: on SIGTERM the client
deliberately leaves allocations running so a restart does not interrupt them
(`allocRunner.Shutdown`), and the executors are reparented to init. Signalling
the agent alone would leave the workload running with nothing tracking it.

Because the launcher talks to the API, it needs reachability on
`127.0.0.1:4646`. It uses `curl` when present, and falls back to
`busybox nc` or `wget`, which is what Android images usually ship.

## Things that differ from Linux

- `raw_exec` must be enabled in the client `options` block. It ships disabled.
- Set `logs { disabled = true }` in job specs. Log collection re-executes the
 nomad binary as `logmon`, and on Android it is safer to log to logcat from
 the application.
- CPU and memory in `resources` are used for scheduling but are **not
 enforced**: there is no usable cgroup controller for `raw_exec`.
- Use `network { mode = "host" }`. Bridge networking needs CNI, which is not
 available.
- The agent does not install a boot hook by itself. Use `nomad.rc` on a
 writable system partition, or your device's own boot mechanism.
