# Running Nomad as an Android app

This directory builds an APK that runs the Nomad agent inside an ordinary
Android application, without root. The agent runs as the app's own uid, so it
is subject to the same sandbox as any other app.

## Why an APK at all

Two deployment shapes need this:

- devices where the system partition cannot be modified and the shell has no
 root, and
- products that must ship as a normal installable app.

The hard part is not starting the agent. It is that Nomad re-executes its own
binary for the per-task executor, logmon, and the template helpers. A binary
inside an APK cannot be executed in place, so the app extracts a copy to its
private storage, marks it executable, and points the agent at it with
`NOMAD_EXECUTABLE` (see `helper/subproc`).

## Build

No Gradle. AGP pins a narrow range of Gradle versions, and this is meant to
build from a plain command-line SDK. `aapt2`, `javac`, `d8`, `zipalign` and
`apksigner` are enough.

```bash
# from the repository root
NOMAD_NO_UI=1 make pkg/android_arm64/nomad
demo/android/apk/build.sh pkg/android_arm64/nomad.so
adb install -r demo/android/apk/build/nomad-android.apk
```

`build.sh` takes the agent binary as its only argument. Set `ABI` for other
architectures (`ABI=x86_64`), and `BUILD_TOOLS` / `PLATFORM` if your SDK has
different versions than the defaults.

## Install and start

```bash
adb install -r demo/android/apk/build/nomad-android.apk
adb shell am start -n com.hashicorp.nomadandroid/.StartActivity
adb logcat -s NomadService
```

The service writes a single-node config on first launch and starts the agent.
The HTTP API listens on `127.0.0.1:4646` inside the device:

```bash
adb forward tcp:4646 tcp:4646
nomad node status
```

## Two constraints that break naive setups

Both were found by running on a device; both are fatal, and neither produces
an obvious error.

### 1. data_dir must be short

A task's secrets directory lives at
`<data_dir>/alloc/<36-char-uuid>/<task>/secrets`, and Nomad creates a unix
socket there for the task API. The kernel caps a socket path at 108 bytes.

An app-private data dir such as `/data/user/0/com.example.app/files` is
already ~45 characters before any of that, which overflows and fails every
allocation with:

```
error creating task api socket: bind: invalid argument
```

The app therefore uses `/data/local/tmp/nd`, and the directory has to exist
and be writable by the app before the service starts:

```bash
adb shell mkdir -p /data/local/tmp/nd /data/local/tmp/alloc_mounts
adb shell chmod 777 /data/local/tmp/nd /data/local/tmp/alloc_mounts
```

`alloc_mounts` is a fixed path under the client mounts dir and cannot be
relocated, so it needs the same treatment. Both are world-writable, which is
acceptable on a device that already runs untrusted apps as separate uids, but
worth revisiting if you control the ROM.

### 2. TMPDIR must be writable

go-plugin creates its socket with `os.CreateTemp`, which honours `TMPDIR`.
Android resolves that to `/data/local/tmp`, where an app has no write access.
The executor starts, writes one line to its log, and exits:

```
plugin init error: open /data/local/tmp/plugin1156988624: permission denied
```

From the agent's point of view this looks like a failed handshake, and the
error it reports ("the plugin was not compiled for this architecture") is
actively misleading. The service sets `TMPDIR` to a directory under
`data_dir`.

The executor's own log is the place to look when a task will not start:

```bash
adb shell cat /data/local/tmp/nd/alloc/<alloc-id>/<task>/executor.out
```

## What is verified

On an RK3399 board, Android 8.1 (API 27), arm64, SELinux permissive, the app
installed as uid `u0_a119` (no root):

- the agent starts as the app, elects itself leader, and registers the node
 with `raw_exec` as the only usable driver;
- a `raw_exec` allocation reaches `running`, with the process tree
 `com.hashicorp.nomadandroid -> nomad (agent) -> nomad (executor) -> task`.

## Limitations

- `raw_exec` tasks run with the app's uid and, on Android, its seccomp filter
 and SELinux domain. Anything that needs root or a different domain will not
 work.
- The app has no wish to keep the agent alive across reboots; use a boot
 receiver if you need that.
- Ports below 1024 are not available to an app, so the default agent ports
 (4646/4647) are fine but Consul on 8500 or similar would not be.
