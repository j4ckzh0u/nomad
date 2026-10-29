# Nomad on Android

This document describes the Android port of the Nomad client: what was
changed, why, and how to build and run it.

The goal is to let Nomad schedule plain binaries on an Android device
(embedded boards, kiosks, automotive head units) using the `raw_exec` driver,
without Docker or any container runtime.

## Why this is not just a compile fix

`GOOS=android` is not an officially supported Nomad target. Cross-compiling
works out of the box (`//go:build linux` matches Android), and the agent starts
and elects a leader. However, every allocation failed during the pre-run phase:

```
failed to setup alloc: pre-run hook "alloc_dir" failed:
Unable to find nobody user: user: Lookup not implemented on android
```

The root cause is that Android has no user database:

- There is no `/etc/passwd` (verified on a real device).
- User and group ids come from bionic's fixed `AID_*` tables.
- Go's `os/user` package is therefore a stub on Android. It resolves only the
 current user (uid 0 / root) and returns
 `user: Lookup not implemented on android` for every name, including
 `nobody`.

Nomad resolves `nobody` when preparing each allocation directory, and treats a
failure as fatal, so no task could ever start.

## Changes

All changes are backward compatible on supported platforms; the Android
behaviour is only reached when a lookup fails in a way that indicates the
platform has no user database at all.

### `helper/users/lookup.go`

Added `IsLookupUnsupported(err error) bool`, which distinguishes "this platform
has no user database" from "this particular user does not exist". Only the
former should be non-fatal.

### `client/allocdir/fs_unix.go`

`dropDirPermissions` no longer fails the allocation when `nobody` cannot be
resolved on such a platform. The directory has already been widened to `0777`,
so it is simply left owned by the agent user. The effective isolation is
unchanged.

### `client/allocdir/task_dir.go`

The `unveil` filesystem-isolation branch falls back to the agent's own uid/gid
when `nobody` cannot be resolved. This branch is not used by `raw_exec`
(which declares `fsisolation.None`), but the same reasoning applies.

### `drivers/shared/validators/validators_unix.go`

`getGroupsID` falls back to the user's primary gid when group membership cannot
be enumerated, so the `denied_host_uids` / `denied_host_gids` deny-lists still
work for the ids that are visible.

### `helper/pluginutils/catalog/register_android.go`

A new Android-specific plugin catalog registers `raw_exec`, `qemu` and `java`
but **not** `docker`. Android has no Docker daemon and cannot run one, so
registering the driver only produces a permanent "undetected" fingerprint and
repeated connection errors in the logs.

The existing `register.go` (`!cgo`) and `register_cgo.go` (`cgo`) gained a
`&& !android` constraint so exactly one catalog is compiled.

### `client/allocdir/stale_linux.go`

Adds `unmountStaleSecretDirs`, called once per agent process from
`AllocDir.Build`. It unmounts and recreates any `secrets/` and `private/`
tmpfs left over from a previous process that did not shut down cleanly, so
repeated hard restarts do not accumulate leaked mounts.

### `GNUmakefile`

Added `pkg/android_%/nomad`, producing `pkg/android_arm64/nomad.so`. The `.so`
suffix is deliberate: on Android only files inside an APK's `lib/<abi>/`
directory are executable, and that is where such a file must be placed.

## Building

```bash
NOMAD_NO_UI=1 make pkg/android_arm64/nomad
```

`NOMAD_NO_UI=1` skips embedding the web UI, which needs the Ember toolchain.
`CGO_ENABLED` is forced to 0; the `exec` driver is not built because it
requires cgo and libcontainer-based chroot isolation.

## Running

### Where to put the binary

This matters more than any code change. Android only allows executing files in
a few locations:

| Location | Executable | Notes |
| --- | --- | --- |
| `/system/bin/` | yes | needs a writable system partition (root / custom ROM) |
| `lib/<abi>/` inside an APK | yes | with `android:extractNativeLibs="false"` |
| `/data/local/tmp/` | only when SELinux is permissive or the device is rooted | |

Android 10 and later block `exec` from app-private storage under `/data` at the
SELinux level for ordinary apps. On the test device (Android 8.1, `userdebug`,
SELinux permissive, `adb shell` running as root) `/data/local/tmp` worked, but
do not rely on that for production.

Because Nomad re-executes its own binary to start the per-task `executor` and
`logmon` helpers, the binary must stay executable for the lifetime of the
agent, not just at startup.

### Telling Nomad where its binary is

When the running image is not executable, for example because the agent was
launched from `lib/<abi>/libnomad.so` inside an APK, the helpers need an
explicit path. `helper/subproc` resolves it in this order:

1. `executable_path` in the agent config.
2. `$NOMAD_EXECUTABLE`.
3. `os.Executable()`.
4. Android only, when the running image is not executable:
 `$NOMAD_ANDROID_LIB_DIR/libnomad.so`, `/system/bin/nomad`,
 `/system/xbin/nomad`, `/data/local/tmp/nomad`.

Candidates are checked for existence and an execute bit before use, so a bad
configuration fails immediately with a clear error rather than an obscure
`exec` failure on the first job. The agent logs a warning when the helper
binary differs from the running image.

```hcl
executable_path = "/data/local/nomad/nomad"
```

See `demo/android/DEPLOYMENT.md` for the three deployment shapes (rooted
board, enforcing device without root, and multi-process with rolling
updates).

### Configuration

```hcl
data_dir = "/data/local/nomad/data"
bind_addr = "0.0.0.0"
log_level = "INFO"

server {
 enabled = true
 bootstrap_expect = 1
}

client {
 enabled = true
 options = {
 "driver.raw_exec.enable" = "1"
 }
}
```

`raw_exec` is disabled by default in Nomad and must be enabled explicitly.

### Job file

```hcl
job "android-svc" {
 datacenters = ["dc1"]
 type = "service"

 group "svc" {
 network {
 mode = "host"
 }

 task "svc" {
 driver = "raw_exec"

 config {
 command = "/data/local/services/myservice"
 }

 # logmon is a re-exec helper; keep log collection off unless the
 # binary's location is guaranteed executable. Write to logcat instead.
 logs {
 disabled = true
 }

 restart {
 attempts = 3
 interval = "5m"
 delay = "5s"
 mode = "fail"
 }
 }
 }
}
```

## Verified behaviour

Tested on an RK3399 board, Android 8.1 (API 27), arm64, kernel 4.4.167,
SELinux permissive, running as root:

- `nomad agent` starts, elects itself leader, registers the node.
- `raw_exec` fingerprints as healthy; `docker` is absent from the driver list.
- A `raw_exec` allocation reaches `running` and the deployment reports
 `healthy`.
- The process tree is `nomad (agent) -> nomad (executor) -> service`, so the
 executor re-exec works on this platform.
- `nomad job stop` terminates both the executor and the service process.

## Known limitations

- **Single node only in practice.** A multi-node cluster needs Raft, Serf and
 several ports; battery and background-execution policy on Android make this
 fragile.
- **Log collection.** `logmon` is another re-exec helper. `logs { disabled =
 true }` is recommended; use logcat for application logs.
- **No resource isolation.** `raw_exec` provides no cgroup enforcement on
 Android, and cgroups v1 handling is best-effort. CPU and memory limits in
 the job spec are not enforced.
- **No CSI, no Consul Connect, no Vault** in the tested configuration. These
 depend on infrastructure that is not present on a bare device.
- **Unclean shutdown leaves tmpfs mounts until the next allocation.** If the
 agent is killed with `SIGKILL`, the `secrets/` and `private/` tmpfs mounts
 inside alloc directories stay behind. The agent sweeps and unmounts them once
 per process before the first allocation is built, so they are reclaimed as
 soon as scheduling resumes, but they do survive until then. Prefer `SIGTERM`
 and let the client drain.
- **`codegen_generated` tag** does not build in this repository state; the
 Android make target therefore overrides `GO_TAGS` instead of appending.
