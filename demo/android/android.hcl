# Single-node Nomad agent for Android.
#
# Validated on an RK3399 board running Android 8.1 (API 27), arm64.
# See contributing/android.md for what this port changes and why.

data_dir = "/data/local/nomad/data"
bind_addr = "0.0.0.0"
log_level = "INFO"

# Do not attempt to reach HashiCorp's release checker: these devices are
# frequently offline, and the failure only adds noise to the log.
disable_update_check = true

server {
 enabled = true
 bootstrap_expect = 1
}

client {
 enabled = true

 # raw_exec is disabled by default and is the only useful driver here.
 options = {
 "driver.raw_exec.enable" = "1"
 }

 # Host volume for the binaries you intend to run. Create the directory on
 # the device before starting the agent.
 host_volume "services" {
 path = "/data/local/services"
 read_only = false
 }
}

telemetry {
 disable_hostname = true
 prometheus_metrics = false
}
