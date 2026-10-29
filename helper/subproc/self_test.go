// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: BUSL-1.1

package subproc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shoenig/test/must"
)

// writeExecutable creates a file with the given mode and returns its path.
func writeExecutable(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
	return path
}

func TestSetExecutableOverride_UsedBySelf(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "nomad", 0o755)

	// reset any prior state
	t.Cleanup(func() { SetExecutableOverride("") })
	SetExecutableOverride(bin)

	if got := Self(); got != bin {
		t.Fatalf("Self() = %q, want %q", got, bin)
	}
}

func TestSetExecutableOverride_EmptyClears(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "nomad", 0o755)

	t.Cleanup(func() { SetExecutableOverride("") })
	SetExecutableOverride(bin)
	if got := Self(); got != bin {
		t.Fatalf("Self() = %q, want %q", got, bin)
	}

	// Clearing the override must fall back to the running image. In a test
	// binary that is the test executable, which exists.
	SetExecutableOverride("")
	if got := Self(); got == bin {
		t.Fatalf("Self() still returns the override %q after clearing", bin)
	}
}

func TestSetExecutableOverride_RejectsNonExecutable(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "nomad", 0o644)

	t.Cleanup(func() { SetExecutableOverride("") })
	SetExecutableOverride(bin)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic for a non-executable override")
		}
		if !strings.Contains(fmt.Sprint(r), "no execute permission") {
			t.Fatalf("unexpected panic message: %v", r)
		}
	}()
	Self()
}

func TestSetExecutableOverride_RejectsMissing(t *testing.T) {
	t.Cleanup(func() { SetExecutableOverride("") })
	SetExecutableOverride(filepath.Join(t.TempDir(), "absent"))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a panic for a missing override")
		}
	}()
	Self()
}

func TestSetExecutableOverride_RejectsDirectory(t *testing.T) {
	t.Cleanup(func() { SetExecutableOverride("") })
	SetExecutableOverride(t.TempDir())

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a panic when the override is a directory")
		}
	}()
	Self()
}

func TestCheckExecutable(t *testing.T) {
	dir := t.TempDir()

	exe := writeExecutable(t, dir, "exe", 0o755)
	if err := checkExecutable(exe); err != nil {
		t.Fatalf("expected %q to be executable: %v", exe, err)
	}

	noExec := writeExecutable(t, dir, "noexec", 0o644)
	if err := checkExecutable(noExec); err == nil {
		t.Fatalf("expected %q to be rejected", noExec)
	}

	if err := checkExecutable(dir); err == nil {
		t.Fatal("expected a directory to be rejected")
	}

	if err := checkExecutable(filepath.Join(dir, "absent")); err == nil {
		t.Fatal("expected a missing file to be rejected")
	}
}

func TestExecutablePath_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "nomad", 0o755)

	t.Cleanup(func() { SetExecutableOverride("") })
	SetExecutableOverride("")
	t.Setenv("NOMAD_EXECUTABLE", bin)

	got, err := executablePath()
	if err != nil {
		t.Fatalf("executablePath() error: %v", err)
	}
	if got != bin {
		t.Fatalf("executablePath() = %q, want %q", got, bin)
	}
}

func TestExecutablePath_ExplicitOverrideBeatsEnv(t *testing.T) {
	dir := t.TempDir()
	explicit := writeExecutable(t, dir, "explicit", 0o755)
	fromEnv := writeExecutable(t, dir, "fromenv", 0o755)

	t.Cleanup(func() { SetExecutableOverride("") })
	t.Setenv("NOMAD_EXECUTABLE", fromEnv)
	SetExecutableOverride(explicit)

	got, err := executablePath()
	if err != nil {
		t.Fatalf("executablePath() error: %v", err)
	}
	if got != explicit {
		t.Fatalf("executablePath() = %q, want the explicit override %q", got, explicit)
	}
}

func TestSelf(t *testing.T) {
	value := Self()
	must.NotEq(t, "", value)
}

func TestSelfOverrideBeatsTestBinaryLookup(t *testing.T) {
	// Regression guard: Self() rewrites a ".test" image to whatever nomad is
	// first in $PATH. That must not apply when an explicit override is set,
	// otherwise the override is silently ignored in tests and in embedding
	// hosts.
	dir := t.TempDir()
	name := "nomad-test-image" + ".test"
	bin := writeExecutable(t, dir, name, 0o755)

	t.Cleanup(func() { SetExecutableOverride("") })
	SetExecutableOverride(bin)

	if got := Self(); got != bin {
		t.Fatalf("Self() = %q, want the override %q", got, bin)
	}
}

func TestHasOverride(t *testing.T) {
	t.Cleanup(func() { SetExecutableOverride("") })

	SetExecutableOverride("")
	if hasOverride() {
		t.Fatal("expected no override after clearing")
	}

	dir := t.TempDir()
	bin := writeExecutable(t, dir, "nomad", 0o755)
	SetExecutableOverride(bin)
	if !hasOverride() {
		t.Fatal("expected an override to be reported")
	}
}
