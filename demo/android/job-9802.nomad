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
 # The interpreter is the command and the script is an argument. Pointing
 # command at the script itself fails here with "could not be found" even
 # though the path exists and is executable.
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
