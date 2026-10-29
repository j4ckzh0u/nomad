# Nomad on Android: deployment shapes

Three deployment shapes cover the situations that actually occur. Pick one
before you start; the code supports all of them.

## 1. Development board, root available

Typical of RK3399/RK3588 boards, kiosks, automotive head units, and anything
running a `userdebug` or `eng` build.

**Where the binary goes:** `/system/bin/nomad`, or anywhere under `/data` if
SELinux is permissive.

**Executability:** works because init and adb shell run as root. On
`userdebug` builds SELinux is often permissive, which is what allows exec from
`/data/local/tmp`.

**Start it:**

```sh
adb push pkg/android_arm64/nomad.so /data/local/tmp/nomad
adb shell chmod 755 /data/local/tmp/nomad
adb push demo/android/android.hcl /data/local/nomad/android.hcl
adb push demo/android/nomad-android.sh /data/local/nomad/nomad-android.sh
adb shell sh /data/local/nomad/nomad-android.sh start
```

For boot persistence install the init service:

```sh
adb root && adb remount
adb push pkg/android_arm64/nomad.so /system/bin/nomad
adb shell chmod 755 /system/bin/nomad
adb shell mkdir -p /data/local/nomad /data/local/services
adb push demo/android/android.hcl /data/local/nomad/android.hcl
adb push demo/android/nomad.rc /system/etc/init/nomad.rc
adb shell chmod 644 /system/etc/init/nomad.rc
adb reboot
```

Verify with:

```sh
adb shell getprop init.svc.nomad # running
adb shell 'ps -A -o PID,PPID,NAME | grep nomad' # pid, ppid 1
```

Two things init does not report clearly, both found by rebooting and reading
`dmesg`:

- The service must declare a `seclabel`. Without one, init parses the file and
 evaluates its actions, then logs `service nomad does not have a SELinux
 domain defined` and never starts anything. `u:r:su:s0` is the permissive
 domain available on userdebug builds; a production device needs its own.
- `on init` actions run in file order, so the `mkdir` block has to come before
 the service, and the mode is plain octal (`777`), not `0777`.


## 2. Production device, enforcing SELinux, no root

Typical of commercial tablets, phones, and locked-down appliances. Android10+
refuses to exec anything from app-private storage, so the binary cannot simply
be pushed to `/data`.

**Where the binary goes:** `lib/<abi>/libnomad.so` inside your APK, with
`android:extractNativeLibs="false"` in the manifest.

**The catch:** a file packaged that way is memory-mapped out of the APK, so
`os.Executable()` yields an APK-internal path that `execve` cannot use. The
agent still needs to re-execute itself to start the per-task executor, logmon,
and template helpers.

**The fix:** on first launch, copy the payload out of the APK into a location
the app may execute from, then tell Nomad where it is. The supported
mechanisms, in priority order:

1. `executable_path` in the agent config.
2. `NOMAD_EXECUTABLE` in the environment.
3. `NOMAD_ANDROID_LIB_DIR`, whose `libnomad.so` is probed automatically.
4. The built-in probes: `/system/bin/nomad`, `/system/xbin/nomad`,
 `/data/local/tmp/nomad`.

A minimal Kotlin bootstrap:

```kotlin
class NomadService : Service() {
 override fun onCreate() {
 // Copy the packaged binary to a path we are allowed to exec from.
 val bin = File(filesDir, "bin/nomad")
 if (!bin.exists()) {
 resources.openRawResource(R.raw.nomad).use { input ->
 bin.parentFile?.mkdirs()
 bin.outputStream().use { input.copyTo(it) }
 }
 bin.setExecutable(true)

 // Nomad re-executes itself for helpers; point it at that copy.
 val env = mapOf("NOMAD_EXECUTABLE" to bin.absolutePath)
 ProcessBuilder(bin.absolutePath, "agent", "-config", confPath)
 .redirectErrorStream(true)
 .also { it.environment().putAll(env) }
 .start()
 }

 override fun onBind(intent: Intent?) = null
}
```

Note that `filesDir` is under `/data`, which is exactly what Android10+ blocks.
Whether this works depends on the device: exec restrictions key off the SELinux
domain the app runs in. If your app's domain cannot exec from its data dir,
ship the binary in `lib/<abi>/` and copy it to `/data/local/tmp`, which is
executable on most devices, or use a device-owner / system-app deployment
where the domain permits it.

**Recommendation:** validate this on the actual device early. The exec policy
is per-device and per-domain; there is no single answer that is correct for
every Android build.

## 3. Multi-process, rolling updates

Once the agent runs, this shape is identical to Linux Nomad:

- `demo/android/stack.nomad` shows a two-instance service with a rolling
 update, a host volume, and a restart policy.
- `artifact { }` works with `raw_exec`, so binaries can be delivered by HTTP
 instead of adb. This is the practical answer to "how do I update a service
 on fifty devices".
- Be aware that CPU and memory in `resources { }` are **not enforced** under
 `raw_exec` on Android, because there is no usable cgroup controller. They
 are still used for scheduling decisions.

## Checking which shape you are in

```sh
# Is SELinux enforcing?
adb shell getenforce

# Can the shell exec from /data?
adb shell 'cp /system/bin/echo /data/local/tmp/t && chmod 755 /data/local/tmp/t && /data/local/tmp/t ok'

# What image path does the agent see?
adb shell '/data/local/tmp/nomad version'
```

If `getenforce` returns `Enforcing` and the exec test fails, you are in shape 2
and need the `executable_path` / `NOMAD_EXECUTABLE` mechanism.
