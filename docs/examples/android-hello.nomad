job "android-hello" {
 datacenters = ["dc1"]
 type = "batch"

 group "g" {
 network {
 mode = "host"
 }

 task "t" {
 driver = "raw_exec"

 config {
 command = "/system/bin/sh"
 args = ["-c", "getprop ro.product.model; getprop ro.build.version.release; id"]
 }

 # Log collection re-executes the agent as logmon, which is fragile on
 # Android. Leave it off and read the exit code instead, or have the
 # application write to logcat.
 logs {
 disabled = true
 }

 kill_timeout = "2s"
 }
 }
}
