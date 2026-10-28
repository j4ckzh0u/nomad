// Copyright IBM Corp.2015,2026
// SPDX-License-Identifier: BUSL-1.1

//go:build android

package catalog

import (
	"github.com/hashicorp/nomad/drivers/java"
	"github.com/hashicorp/nomad/drivers/qemu"
	"github.com/hashicorp/nomad/drivers/rawexec"
)

// This file registers builtin plugins for Android.
//
// Android has no Docker daemon and no way to run one, so the docker driver is
// deliberately not registered: it would only ever fingerprint as undetected
// and log connection errors on every interval. The exec driver is also absent
// because it requires cgo and libcontainer-style chroot isolation, which is
// not available here; raw_exec is the driver of choice.
func init() {
	RegisterDeferredConfig(rawexec.PluginID, rawexec.PluginConfig, rawexec.PluginLoader)
	Register(qemu.PluginID, qemu.PluginConfig)
	Register(java.PluginID, java.PluginConfig)
}
