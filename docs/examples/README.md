# Examples

Each example is a complete job file. All of them were run on real hardware;
the platform each one targets is noted.

| File | Type | Runs on | What it proves |
| --- | --- | --- | --- |
| [hello-unix.nomad](hello-unix.nomad) | batch | Linux, macOS | The scheduler places work and the driver starts a process |
| [hello-windows.nomad](hello-windows.nomad) | batch | Windows | The same, with a Windows interpreter path |
| [android-hello.nomad](android-hello.nomad) | batch | Android | `raw_exec` works on a device without Docker |
| [android-service.nomad](android-service.nomad) | service | Android | Restart policy and a long running task |
| [../../demo/android/job-9801.nomad](../../demo/android/job-9801.nomad) | service | Android | Two HTTP services, ports 9801 and 9802 |

## Run one

```sh
nomad job run docs/examples/hello-unix.nomad
nomad job status hello-unix
nomad job allocs hello-unix
```

For a batch job the allocation finishes on its own. Read its output:

```sh
nomad alloc logs <alloc-id> t
```

If the task has `logs { disabled = true }` there is nothing to read, which is
the usual setting on Android. Check the exit code instead:

```sh
nomad job allocs hello-unix -t '{{range .}}{{.ID}} {{.ClientStatus}}{{println}}{{end}}'
```

## The two-service test

The most useful smoke test on Android, because it shows two instances being
placed independently and both reachable.

```sh
adb push demo/android/httpserver/respond.sh demo/android/httpserver/serve-9801.sh \
 demo/android/httpserver/serve-9802.sh /data/local/tmp/
adb shell chmod 755 /data/local/tmp/respond.sh /data/local/tmp/serve-9801.sh /data/local/tmp/serve-9802.sh

nomad job run demo/android/job-9801.nomad
nomad job run demo/android/job-9802.nomad

curl http://<device-ip>:9801/ # svc-a
curl http://<device-ip>:9802/ # svc-b
```

An empty reply or a reset does not mean the job failed. The listener is
`toybox nc`, which serialises connections; retry once before investigating.
Check the job state first:

```sh
nomad job status
```

## Writing your own

Minimum viable service job:

```hcl
job "myapp" {
 datacenters = ["dc1"]
 type = "service"

 group "g" {
 network {
 mode = "host"
 }

 task "app" {
 driver = "raw_exec"

 config {
 command = "/path/to/application"
 # args = ["--flag"]
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

`raw_exec` runs the process as the agent's user with no filesystem isolation.
That is the point on Android, where there is no container runtime, and it means
the job file is trusted input: anyone who can submit jobs can run anything the
agent can.

For a binary that is a shell script, invoke the interpreter and pass the script
as an argument. Setting `command` to the script itself can fail with "could not
be found" even when the path is correct; see
[../troubleshooting.md](../troubleshooting.md#command-set-to-a-script-path-fails).
