job "http-9802" {
 datacenters = ["dc1"]
 type = "service"

 group "web" {
 network {
 mode = "host"
 }

 task "srv" {
 driver = "raw_exec"

 config {
 # The interpreter is the command and the script is an argument: that does not
 # depend on the executable bit, and a missing script is reported by the shell
 # instead of by the executor. See docs/troubleshooting.md.
 command = "/system/bin/sh"
 args = ["/data/local/tmp/serve-9802.sh"]
 }

 logs {
 disabled = true
 }

 restart {
 attempts = 3
 interval = "5m"
 delay = "5s"
 }
 }
 }
}
