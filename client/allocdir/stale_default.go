// Copyright IBM Corp.2015,2026
// SPDX-License-Identifier: BUSL-1.1

//go:build !linux

package allocdir

// unmountStaleSecretDirs is a no-op on platforms that do not implement the
// tmpfs backed secret directories.
func unmountStaleSecretDirs(string, map[string]bool) {}
