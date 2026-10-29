// Copyright IBM Corp.2015,2026
// SPDX-License-Identifier: BUSL-1.1

//go:build linux

package allocdir

import (
	"os"
	"path/filepath"
)

// staleSecretDirNames are the per-task directories that may be backed by a
// tmpfs created by createSecretDir.
var staleSecretDirNames = []string{TaskSecrets, TaskPrivate}

// unmountStaleSecretDirs unmounts tmpfs mounts left behind under allocDir by a
// previous agent process that did not shut down cleanly, skipping the allocs
// named in keep so a live allocation is never disturbed.
//
// Normally AllocDir.Destroy unmounts these, but a SIGKILL (or a power loss)
// skips it and the kernel keeps the mounts until reboot. A later agent process
// then finds non-empty secret directories, skips re-mounting (see
// createSecretDir's marker check), and the mount stays around forever. Since
// the whole point of the tmpfs is that secrets never reach disk, leaking them
// is worth cleaning up at startup.
//
// The alloc dir layout is:
//
//	<allocDir>/<allocID>/<taskGroup>.<task>/secrets
//	<allocDir>/<allocID>/<taskGroup>.<task>/private
//
// Errors are ignored: a directory that is not a mount point is not a problem,
// and failing to clean one must never prevent the agent from starting. A mount
// that is still busy is left in place, which happens when an allocation is
// being torn down concurrently.
func unmountStaleSecretDirs(allocDir string, keep map[string]bool) {
	allocEntries, err := os.ReadDir(allocDir)
	if err != nil {
		return
	}

	for _, allocEntry := range allocEntries {
		if !allocEntry.IsDir() || keep[allocEntry.Name()] {
			continue
		}
		allocPath := filepath.Join(allocDir, allocEntry.Name())
		taskEntries, err := os.ReadDir(allocPath)
		if err != nil {
			continue
		}

		for _, taskEntry := range taskEntries {
			if !taskEntry.IsDir() {
				continue
			}
			taskPath := filepath.Join(allocPath, taskEntry.Name())
			for _, name := range staleSecretDirNames {
				dir := filepath.Join(taskPath, name)
				if _, err := os.Stat(dir); err != nil {
					continue
				}
				// unlinkDir ignores EINVAL, i.e. "not a mount point".
				_ = unlinkDir(dir)
				if err := os.RemoveAll(dir); err != nil {
					continue
				}
				_ = os.MkdirAll(dir, fileMode777)
			}
		}
	}
}
