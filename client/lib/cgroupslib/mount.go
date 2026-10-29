// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: BUSL-1.1

//go:build linux

package cgroupslib

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hashicorp/go-set/v3"
)

// detect tries to detect which cgroups version we have by looking at the mount
// and whether Nomad owns the cgroup.
// - For cgroups v1 this requires root.
// - For cgroups v2 we look for root or whether we're the owner of the slice.
// - All other cases, including any file permission errors, return OFF.
func detect() Mode {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return OFF
	}
	defer func() {
		_ = f.Close()
	}()

	mode := scan(f)

	if mode == CG1 && os.Geteuid() > 0 {
		return OFF
	}

	if mode == CG2 {
		if !functionalCgroups2("cgroup.controllers") {
			return OFF
		}
		uid := os.Geteuid()
		if uid > 0 {
			// allow for cgroup delegation if we own the slice
			cgPath := filepathCG("nomad.slice")
			fi, err := os.Stat(cgPath)
			if err != nil {
				return OFF
			}
			if uid != int(fi.Sys().(*syscall.Stat_t).Uid) {
				return OFF
			}
		}
	}

	return mode
}

func functionalCgroups2(controllersFile string) bool {
	requiredCgroup2Controllers := []string{"cpuset", "cpu", "io", "memory", "pids"}

	controllersRootPath := filepath.Join(root, controllersFile)
	content, err := os.ReadFile(controllersRootPath)
	if err != nil {
		return false
	}

	rootSubtreeControllers := set.From[string](strings.Fields(string(content)))
	return rootSubtreeControllers.ContainsSlice(requiredCgroup2Controllers)
}

// scan determines which cgroups version is mounted at /sys/fs/cgroup.
//
// It must not return on the first matching line. Android and container images
// commonly layer several mounts on the same mountpoint: the outer entry is a
// tmpfs (with cgroup v1 controllers mounted beneath it) and a cgroup2 entry is
// bind mounted over the top. Only the last entry for the mountpoint is the
// one actually visible, so keep scanning and let later lines win.
//
// A tmpfs at /sys/fs/cgroup with no cgroup2 above it still means cgroups v1,
// because that is how the v1 hierarchy is exposed.
func scan(in io.Reader) Mode {
	var mode Mode = OFF
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		tokens := set.From(strings.Fields(scanner.Text()))
		if !tokens.Contains("/sys/fs/cgroup") {
			continue
		}
		switch {
		case tokens.Contains("cgroup2"):
			mode = CG2
		case tokens.Contains("tmpfs"):
			// Only downgrade to v1 if no cgroup2 has been seen; a later
			// tmpfs cannot hide an earlier cgroup2 mount in practice.
			if mode != CG2 {
				mode = CG1
			}
		case tokens.Contains("cgroup"):
			// A v1 controller mount ("cgroup cgroup rw,cpuset") at this path.
			if mode == OFF {
				mode = CG1
			}
		}
	}
	return mode
}
