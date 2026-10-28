job "android-example" {
 datacenters = ["dc1"]
 type = "service"

 group "example" {
 # Android has no bridge networking; use the host network.
 network {
 mode = "host"
 }

 task "service" {
 driver = "raw_exec"

 config {
 # Point this at a binary that exists on the device.
 command = "/data/local/services/myservice"
 }

 # logmon is a re-exec helper; on Android it is simpler to write to
 # logcat from the application itself.
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
