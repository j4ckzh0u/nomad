# Troubleshooting

Every entry here was hit on real hardware while building this fork. Each one
gives the exact error, the cause, and the fix.

Android problems are listed first because they are the least familiar. Where a
message is actively misleading, that is called out.

## Android

### `Unable to find nobody user`

```
failed to setup alloc: pre-run hook "alloc_dir" failed:
Unable to find nobody user: user: Lookup not implemented on android
```

**Cause.** Android has no user database. There is no `/etc/passwd`, uids come
from bionic's `AID_*` tables, and Go's `os/user` is a stub that resolves only
the current user. Nomad resolved `nobody` while preparing every allocation
directory and treated failure as fatal, so no task could ever start.

**Fix.** This fork distinguishes "no user database" from "no such user" and
degrades: the directory stays owned by the agent, which is what it was going to
be anyway since it is already mode 0777.

If you are on an older build, no job will run on Android. That is the first
thing to check.

**Verify.** `nomad job status` shows `running` instead of `failed` at
`Task Setup`.

### Helpers cannot start

```
failed to create executor: error creating rpc client for executor plugin:
Unrecognized remote plugin message:
Failed to read any lines from plugin's stdout
This usually means
 the plugin was not compiled for this architecture,
 the plugin is missing dynamic-link libraries necessary to run,
 the plugin is not executable by this process due to file permissions, or
 the plugin failed to negotiate the initial go-plugin protocol handshake
```

Every word of that message is wrong on Android. The binary is the right
architecture and the permissions are fine. Two different causes produce it.

**Cause A: the running image is not executable.** Nomad re-executes itself to
start the executor and logmon. Inside an APK the image is mapped from the APK
and has no usable path.

**Fix A.** Tell it where a real copy lives:

```hcl
executable_path = "/data/local/tmp/nomad"
```

or set `NOMAD_EXECUTABLE` in the environment. The agent logs a warning when the
helper binary differs from the running image, which confirms the override took.

**Cause B: `TMPDIR` is not writable.** go-plugin creates its socket with
`os.CreateTemp`, and Android resolves `TMPDIR` to `/data/local/tmp`, where an
app has no write permission.

**Fix B.** Point `TMPDIR` at a directory the agent owns before starting it:

```sh
TMPDIR=/data/user/0/<pkg>/files/tmp nomad agent -config ...
```

**How to tell them apart.** The executor's own log holds the real reason and is
the only place it appears:

```sh
adb shell cat <data_dir>/alloc/<alloc-id>/<task>/executor.out
```

A line like `plugin init error: open /data/local/tmp/plugin 123: permission
denied` is cause B. If the file exists but is empty, the process never started
and the cause is A.

### `bind: invalid argument` creating the task socket

```
error creating task api socket: listen unix .../secrets/api.sock: bind: invalid argument
```

**Cause.** A unix socket path is capped at 108 bytes by the kernel. A task's
secrets directory is `<data_dir>/alloc/<36-char-uuid>/<task>/secrets`, which
overflows when `data_dir` is long. An app-private path such as
`/data/user/0/com.example.app/files` is already around 45 characters before any
of that.

**Fix.** Use a short `data_dir`, for example `/data/local/tmp/nd`. Two
directories must exist and be writable:

```sh
adb shell mkdir -p /data/local/tmp/nd /data/local/tmp/alloc_mounts
adb shell chmod 777 /data/local/tmp/nd /data/local/tmp/alloc_mounts
```

`alloc_mounts` is a fixed path under the client mounts directory and cannot be
relocated, so it needs the same treatment.

**Check the length** before deploying:

```sh
python3 -c "print(len('/data/local/tmp/nd/alloc/' + '0'*36 + '/t/secrets/api.sock'))"
```

Anything above 108 will fail on the first allocation.

### `failed to detect memset`

```
failed to initialize process manager: failed to detect memset:
open /sys/fs/cgroup/cpuset/cpuset.mems: no such file or directory
```

**Cause.** The image layers a cgroup2 mount over a cgroup v1 hierarchy. The
detection code returned on the first `/sys/fs/cgroup` entry in
`/proc/self/mountinfo`, saw the outer tmpfs, concluded cgroups v1, and then
tried to read a path that the cgroup2 mount had shadowed.

Observed on redroid Android12 x86_64. Pure v1 images such as Android8.1 on
RK3399 do not show it.

**Fix.** The scan now reads the whole file and uses the last entry for the
path, because that is the mount actually visible. Verified by the client
reporting cgroups v2 and creating `/sys/fs/cgroup/nomad.slice`.

**Confirm** on a live device:

```sh
adb shell cat /proc/self/mountinfo | grep /sys/fs/cgroup
```

If `/sys/fs/cgroup` appears more than once, this is your problem.

### `init` never starts the service

The agent is not running after reboot and `getprop init.svc.nomad` is empty,
while `dmesg` shows:

```
init: service nomad does not have a SELinux domain defined
```

**Cause.** A service under `/system/etc/init` must declare `seclabel`. Without
it init parses the file, evaluates its actions, and then refuses to start the
service. Nothing else is logged at normal verbosity, which makes it look like
the file was ignored.

**Fix.** Add the line, using the permissive domain available on userdebug
builds:

```
service nomad /system/bin/nomad agent -config=/data/local/nomad/android.hcl
 class late_start
 user root
 group root system
 seclabel u:r:su:s0
```

A production device needs its own domain and sepolicy; `su` exists only on
development builds.

Two related mistakes, both silent:

- `on init` blocks must appear **before** the service in file order, because
 init evaluates in order.
- The `mkdir` mode is plain octal: `777`, not `0777`.

**Verify** with `adb shell dmesg | grep nomad`. You should see
`Parsing file /system/etc/init/nomad.rc` and later `processing action`.

### tmpfs mounts pile up

After a hard restart, old allocation directories keep their `secrets` and
`private` tmpfs mounts:

```sh
adb shell grep -c 'nomad/data/alloc' /proc/mounts
```

**Cause.** `AllocDir.Destroy` unmounts these normally. A `SIGKILL` or power loss
skips it, the kernel keeps the mounts until reboot, and a later agent finds a
non-empty secrets directory (because of the `.nomad-mount` marker) and skips
re-mounting. The mount is then never reused or released.

**Fix.** This fork sweeps them once per allocation: anything under the alloc
directory that is not the allocation being built is unmounted and recreated.
A mount that is still busy is left alone.

**Manual cleanup**, if you are on an older build:

```sh
adb shell 'grep "nomad/data/alloc" /proc/mounts | while read -r l; do umount $(echo "$l" | cut -d" " -f2); done'
```

### `stop` leaves the workload running

You stop the agent, `ps` still shows the task, and the port is still bound.

**Cause.** Not a bug. The client deliberately leaves allocations alone on
`SIGTERM` so a restart does not interrupt them, and executors are reparented to
init. `allocRunner.Shutdown` says so in its own comment.

**Fix.** Stop the jobs first, through the API:

```sh
nomad job stop <job>
```

The launcher in `demo/android` does this before signalling the agent. Killing
the agent alone, or `pkill nomad`, will not stop work.

### `logmon` is unreliable

Task log collection works by re-executing the agent binary as `logmon`. On
Android that indirection is fragile.

**Fix.** Disable it per task and let the application log elsewhere:

```hcl
task "svc" {
 driver = "raw_exec"
 logs {
 disabled = true
 }
}
```

The application can write to `logcat`; `adb logcat -s <tag>` then becomes the
log viewer.

### `command` set to a script path fails

```
failed to launch command with executor: rpc error: code = Unknown desc =
binary "/data/local/tmp/serve-9801.sh" could not be found
```

The path exists, is mode 0755, and is owned by root. Running it by hand works.

**Fix.** Invoke the interpreter and pass the script as an argument:

```hcl
config {
 command = "/system/bin/sh"
 args = ["/data/local/tmp/serve-9801.sh"]
}
```

This form is reliable. The root cause of the direct form failing was not
isolated; if you hit it with a different layout, treat it as a bug worth
reporting rather than as expected behaviour.

## All platforms

### Node is `ready` but jobs fail feasibility

```
no nodes available to place the task group
```

**Cause.** A node reports `ready` before all drivers have fingerprinted.
Submitting in that window can fail with missing-driver errors.

**Fix.** Wait a few seconds, or check the driver list first:

```sh
nomad node status -self
```

### `job stop` then `job run` reuses an old allocation

Old allocations stay visible as `complete` after a stop. That is garbage
collection lag, not a resurrected task.

**Fix.** Nothing. Filter on `Status` if you are scripting:

```sh
nomad job allocs <job> -t '{{range .}}{{if eq .ClientStatus "running"}}{{.ID}}{{end}}{{end}}'
```

### Binary will not run on macOS

```
Bad CPU type in executable
```

**Cause.** The wrong archive. `darwin_amd64` will not run on Apple Silicon
without Rosetta, and `darwin_arm64` will not run on Intel at all.

**Fix.** Check the machine and download the matching archive:

```sh
uname -m
```

### macOS refuses to open the binary

```
"nomad" cannot be opened because the developer cannot be verified
```

**Cause.** The archives are not signed or notarised, so macOS quarantines them.

**Fix.**

```sh
xattr -d com.apple.quarantine /usr/local/bin/nomad
```

### Windows blocks the binary on first run

**Cause.** Files downloaded with `Invoke-WebRequest` carry a mark-of-the-web.

**Fix.** `Unblock-File` on the executable, once.

## Building

### `no required module provides package github.com/hashicorp/go-set`

```
nomad/structs/structs.generated.go:12:2: no required module provides package
github.com/hashicorp/go-set
```

**Cause.** `structs.generated.go` is guarded by the `codegen_generated` build
tag and imports `github.com/hashicorp/go-set`, while `go.mod` only has
`go-set/v3`. Any build that sets that tag fails, which includes the default
`make` targets on this tree.

**Fix.** Do not set the tag:

```sh
NOMAD_NO_UI=1 make GO_TAGS=release pkg/linux_amd64/nomad
```

The release workflow pins `GO_TAGS` per target for this reason instead of
appending to the defaults.

### `android/amd64 requires external (cgo) linking`

```
android/amd64 requires external (cgo) linking, but cgo is not enabled
```

**Cause.** The Go toolchain refuses to build android/amd64 and android/386
without cgo, because the result has to link against bionic rather than glibc.

**Fix.** Provide an NDK compiler:

```sh
NDK=$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$(uname | tr 'A-Z' 'a-z')-x86_64
NOMAD_NO_UI=1 make pkg/android_amd64/nomad \
 CC=$NDK/bin/x86_64-linux-android31-clang
```

arm64 and arm build as pure Go and need none of this.

## Reporting something new

Include:

- the output of `nomad version`
- `adb shell getprop ro.build.version.sdk`, `ro.product.cpu.abi`, `getenforce`
- `nomad node status -self`
- the executor log: `<data_dir>/alloc/<alloc>/<task>/executor.out`

That last file holds the real error for anything that fails between the driver
and the task, which is where most Android failures happen.