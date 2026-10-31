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

// UnmountStaleSecretDirs unmounts the per-task tmpfs mounts that were left
// behind under allocRoot by allocations this agent is not going to restore.
//
// Normally AllocDir.Destroy unmounts these, but a SIGKILL (or a power loss)
// skips it and the kernel keeps the mounts until reboot. A later agent process
// then finds a secrets directory that still contains the .nomad-mount marker,
// skips re-mounting it (see createSecretDir), and the tmpfs is leaked for the
// lifetime of the machine, still holding whatever secrets were in it.
//
// The alloc dir layout is:
//
//	<allocRoot>/<allocID>/<taskGroup>.<task>/secrets
//	<allocRoot>/<allocID>/<taskGroup>.<task>/private
//
// keep must name every allocation under allocRoot that this agent still owns,
// i.e. every one it is about to restore. A mount can belong to a task that is
// still running from a previous agent process, because the client does not
// stop tasks when it shuts down, so those allocations must stay untouched.
// Anything not in keep is unreachable by this agent and safe to unmount.
//
// The .nomad-mount marker gates the work: it is created inside the tmpfs right
// after the mount, so it is only visible while the mount is in place. A
// directory that is not a mount point — a client that does not run as root
// never creates one — has no marker and is therefore never touched, which
// keeps this function from deleting plain on-disk contents.
//
// A busy mount, i.e. one with open files or a process working directory
// inside it, is left in place rather than forcibly detached. Any other error
// is ignored too: failing to reclaim a mount must never keep the agent from
// starting.
//
// It returns the number of mounts it unmounted.
func UnmountStaleSecretDirs(allocRoot string, keep map[string]bool) int {
	allocEntries, err := os.ReadDir(allocRoot)
	if err != nil {
		return 0
	}

	var unmounted int
	for _, allocEntry := range allocEntries {
		if !allocEntry.IsDir() || keep[allocEntry.Name()] {
			continue
		}
		allocPath := filepath.Join(allocRoot, allocEntry.Name())
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
				if _, err := os.Stat(filepath.Join(dir, secretMarker)); err != nil {
					continue
				}
				// unlinkDir fails with EBUSY when the mount is in use.
				if err := unlinkDir(dir); err != nil {
					continue
				}
				unmounted++
			}
		}
	}

	return unmounted
}
