// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: BUSL-1.1

//go:build linux

package cgroupslib

import (
	"os"
	"strings"
	"testing"

	"github.com/shoenig/test/must"
)

const (
	cg1 = `
33 29 0:27 / /run/lock rw,nosuid,nodev,noexec,relatime shared:6 - tmpfs tmpfs rw,size=5120k
34 25 0:28 / /sys/fs/cgroup ro,nosuid,nodev,noexec shared:9 - tmpfs tmpfs ro,mode=755
35 34 0:29 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime shared:10 - cgroup2 cgroup2 rw,nsdelegate
36 34 0:30 / /sys/fs/cgroup/systemd rw,nosuid,nodev,noexec,relatime shared:11 - cgroup cgroup rw,xattr,name=systemd
`

	// Captured from a redroid Android12 container. /sys/fs/cgroup appears
	// three times: an outer tmpfs, cgroup v1 controllers mounted beneath it,
	// and a cgroup2 bind mount over the whole thing. Only the last one is
	// visible, so the detected mode must be CG2.
	redroid = `
136813670:127 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime shared:511 - tmpfs tmpfs rw,mode=755,inode64
133813650:38 / /dev/blkio rw,nosuid,nodev,noexec,relatime shared:540 - cgroup none rw,blkio
133913680:30 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime shared:541 - cgroup2 none rw,nsdelegate
136913680:31 / /sys/fs/cgroup/systemd rw,nosuid,nodev,noexec,relatime shared:512 master:11 - cgroup cgroup rw,xattr,name=systemd
137313680:37 / /sys/fs/cgroup/cpuset rw,nosuid,nodev,noexec,relatime shared:516 master:18 - cgroup cgroup rw,cpuset
134113650:37 / /dev/cpuset rw,nosuid,nodev,noexec,relatime shared:543 - cgroup none rw,cpuset
`

	cg2 = `
34 28 0:29 / /run/lock rw,nosuid,nodev,noexec,relatime shared:6 - tmpfs tmpfs rw,size=5120k,inode64
35 24 0:30 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime shared:9 - cgroup2 cgroup2 rw,nsdelegate,memory_recursiveprot
36 24 0:31 / /sys/fs/pstore rw,nosuid,nodev,noexec,relatime shared:10 - pstore pstore rw
37 24 0:32 / /sys/firmware/efi/efivars rw,nosuid,nodev,noexec,relatime shared:11 - efivarfs efivarfs rw
`
)

func Test_scan(t *testing.T) {
	cases := []struct {
		name  string
		input string
		exp   Mode
	}{
		{
			name:  "v1",
			input: cg1,
			exp:   CG1,
		},
		{
			name:  "v2",
			input: cg2,
			exp:   CG2,
		},
		{
			name:  "redroid layered v1 under v2",
			input: redroid,
			exp:   CG2,
		},
		{
			name:  "empty",
			input: "",
			exp:   OFF,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(tc.input)
			result := scan(in)
			must.Eq(t, tc.exp, result)
		})
	}
}

func TestGetMode(t *testing.T) {
	mode := GetMode()
	if os.Geteuid() == 0 {
		ok := mode == CG1 || mode == CG2
		must.True(t, ok)
	} else {
		// note: we can't test with a delegated cgroup without overwriting the
		// package level root
		must.Eq(t, OFF, mode)
	}
}
