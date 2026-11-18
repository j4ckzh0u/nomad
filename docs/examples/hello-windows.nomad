# Copyright IBM Corp. 2015, 2026
# SPDX-License-Identifier: BUSL-1.1

job "hello-windows" {
 datacenters = ["dc1"]
 type = "batch"

 group "g" {
 task "t" {
 driver = "raw_exec"

 config {
 # raw_exec does not search %PATH% for command, so the interpreter is
 # spelled out in full.
 command = "C:/Windows/System32/cmd.exe"
 args = ["/c", "echo hello from %COMPUTERNAME% && ver"]
 }

 kill_timeout = "2s"
 }
 }
}