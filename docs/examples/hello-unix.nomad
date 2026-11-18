# Copyright IBM Corp. 2015, 2026
# SPDX-License-Identifier: BUSL-1.1

job "hello-unix" {
 datacenters = ["dc1"]
 type = "batch"

 # A batch job runs once and exits. Nomad records the exit code, and the job
 # moves to dead once every allocation is terminal.
 group "g" {
 task "t" {
 driver = "raw_exec"

 config {
 command = "/bin/sh"
 args = ["-c", "echo hello from $(hostname); uname -s"]
 }

 # Two seconds is plenty here. The default kill timeout is measured in
 # minutes.
 kill_timeout = "2s"
 }
 }
}