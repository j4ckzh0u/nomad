job "android-stack" {
 datacenters = ["dc1"]
 type = "service"

 # Rolling updates: one instance at a time, healthy before continuing.
 update {
 max_parallel = 1
 health_check = "task_states"
 min_healthy_time = "10s"
 healthy_deadline = "3m"
 auto_revert = true
 }

 group "worker" {
 count = 2

 network {
 mode = "host"
 }

 # Reuse a host volume across restarts so application state survives a
 # redeploy. Requires a host_volume block in the client config.
 volume "services" {
 type = "host"
 source = "services"
 read_only = false
 }

 task "worker" {
 driver = "raw_exec"

 # Artifacts are fetched before the task starts, which is how a binary gets
 # onto the device without adb. Point this at your own build server.
 #
 # artifact {
 # source = "https://builds.example.com/worker?checksum=sha256:..."
 # destination = "local/bin"
 # }

 config {
 command = "/data/local/services/worker"
 args = ["--bind", "0.0.0.0:9000"]
 }

 env {
 SERVICE_MODE = "worker"
 LOG_FORMAT = "json"
 }

 # logmon re-executes the nomad binary, which is unreliable on Android;
 # write to logcat from the application instead.
 logs {
 disabled = true
 }

 # Restart policy for a long running service.
 restart {
 attempts =3
 interval = "5m"
 delay = "5s"
 mode = "fail"
 }

 # CPU/memory are recorded for scheduling but are NOT enforced under
 # raw_exec on Android (no cgroup controller). Keep them as documentation.
 resources {
 cpu = 100
 memory = 128
 }
 }
 }
}
