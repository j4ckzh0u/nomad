// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: BUSL-1.1

//go:build linux

package allocdir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/nomad/ci"
	"github.com/shoenig/test/must"
	"golang.org/x/sys/unix"
)

// TestUnmountStaleSecretDirs asserts that the sweep reclaims the tmpfs mounts
// of allocations the agent does not own, and leaves every other directory
// alone.
func TestUnmountStaleSecretDirs(t *testing.T) {
	ci.Parallel(t)

	if unix.Geteuid() != 0 {
		t.Skip("must be run as root")
	}

	root := t.TempDir()

	// An allocation the agent is about to restore: its mount must survive.
	keptSecrets := filepath.Join(root, "alloc-kept", "group.task", TaskSecrets)
	must.NoError(t, os.MkdirAll(filepath.Dir(keptSecrets), 0o755))
	must.NoError(t, createSecretDir(keptSecrets, 1))

	// An allocation the agent no longer knows about: both mounts are leaked
	// and must be reclaimed.
	staleTask := filepath.Join(root, "alloc-stale", "group.task")
	must.NoError(t, os.MkdirAll(staleTask, 0o755))
	must.NoError(t, createSecretDir(filepath.Join(staleTask, TaskSecrets), 1))
	must.NoError(t, createSecretDir(filepath.Join(staleTask, TaskPrivate), 1))

	// A directory that is not a mount point, as a client that does not run as
	// root leaves behind: it must not be touched.
	plainSecrets := filepath.Join(root, "alloc-plain", "group.task", TaskSecrets)
	must.NoError(t, os.MkdirAll(plainSecrets, 0o755))
	plainFile := filepath.Join(plainSecrets, "leftover")
	must.NoError(t, os.WriteFile(plainFile, []byte("keep me"), 0o644))

	// The setup is what the assertions depend on: the two stale directories
	// and the kept one are mounted.
	for _, dir := range []string{keptSecrets, filepath.Join(staleTask, TaskSecrets), filepath.Join(staleTask, TaskPrivate)} {
		_, err := isMount(dir)
		must.NoError(t, err)
	}

	unmounted := UnmountStaleSecretDirs(root, map[string]bool{"alloc-kept": true})
	must.Eq(t, 2, unmounted)

	// The kept mount is still there, and its contents are intact.
	_, err := isMount(keptSecrets)
	must.NoError(t, err)

	// The stale mounts are gone.
	_, err = isMount(filepath.Join(staleTask, TaskSecrets))
	must.ErrorIs(t, err, notFoundErr)
	_, err = isMount(filepath.Join(staleTask, TaskPrivate))
	must.ErrorIs(t, err, notFoundErr)

	// The plain directory still holds its contents.
	content, err := os.ReadFile(plainFile)
	must.NoError(t, err)
	must.Eq(t, "keep me", string(content))

	// Unmount the mount the sweep deliberately left behind so the temp dir can
	// be removed.
	must.NoError(t, removeSecretDir(keptSecrets))
}
