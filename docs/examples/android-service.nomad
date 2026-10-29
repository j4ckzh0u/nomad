job "android-service" {
 datacenters = ["dc1"]
 type = "service"

 # A long running service. Nomad keeps it alive, restarts it if it exits,
 # and reschedules it if the node goes away.
 group "svc" {
 count = 1

 network {
 mode = "host"
 }

 task "svc" {
 driver = "raw_exec"

 config {
 # The binary must be executable in place, or invoked through an
 # interpreter as here. See docs/troubleshooting.md.
 command = "/system/bin/sh"
 args = ["-c", "while true; do echo alive $(date); sleep 30; done"]
 }

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
