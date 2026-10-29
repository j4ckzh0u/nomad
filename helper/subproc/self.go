// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: BUSL-1.1

package subproc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	// executable is the executable of this process
	executable string
	once       sync.Once

	// override, when set, short-circuits executable discovery. See
	// SetExecutableOverride.
	override   string
	overrideMu sync.RWMutex
)

// SetExecutableOverride forces subproc.Self (and therefore every helper that
// re-executes the nomad binary: the task executor, logmon, and the template
// and artifact subprocesses) to use path instead of the running image.
//
// This exists because Android does not always allow executing the file that
// os.Executable() reports:
//
// - Android10+ blocks exec from app-private storage under /data for
// ordinary apps (SELinux), so a binary copied to the app's data dir can
// be loaded but not re-executed.
// - The supported way to ship a native binary inside an APK is
// lib/<abi>/libnomad.so with extractNativeLibs="false". Android maps that
// file from the APK, so its path on disk is an APK-internal offset, not a
// path that can be passed to execve.
//
// In both cases the embedding application must install the binary somewhere
// executable (or extract it at startup) and point Nomad at it once, before
// the client starts.
//
// SetExecutableOverride must be called before the first use of Self; it is
// intended to be called during process startup. An empty path clears the
// override.
func SetExecutableOverride(path string) {
	overrideMu.Lock()
	defer overrideMu.Unlock()
	override = path
	// Force the next Self call to re-resolve.
	once = sync.Once{}
}

// executablePath returns the path used to re-execute this binary.
//
// Order of preference:
// 1. an explicit override (see SetExecutableOverride).
// 2. $NOMAD_EXECUTABLE, which lets an embedding host or an init script fix
// the path when the binary was started from a location that cannot be
// executed again.
// 3. os.Executable().
func executablePath() (string, error) {
	overrideMu.RLock()
	explicit := override
	overrideMu.RUnlock()
	if explicit != "" {
		if err := checkExecutable(explicit); err != nil {
			return "", fmt.Errorf("configured executable %q: %w", explicit, err)
		}
		return explicit, nil
	}

	if env := os.Getenv("NOMAD_EXECUTABLE"); env != "" {
		if err := checkExecutable(env); err != nil {
			return "", fmt.Errorf("NOMAD_EXECUTABLE %q: %w", env, err)
		}
		return env, nil
	}

	bin, err := os.Executable()
	if err != nil {
		return "", err
	}

	// On Android the reported image may not be executable, for the reasons
	// documented on SetExecutableOverride. Fall back to a copy that the
	// packaging step is expected to install.
	if runtime.GOOS == "android" {
		if err := checkExecutable(bin); err != nil {
			for _, candidate := range androidFallbackPaths() {
				if checkExecutable(candidate) == nil {
					return candidate, nil
				}
			}
			return "", fmt.Errorf(
				"nomad binary at %q is not executable and no fallback was found: %w; "+
					"set NOMAD_EXECUTABLE or call subproc.SetExecutableOverride", bin, err)
		}
	}

	return bin, nil
}

// androidFallbackPaths are locations that can be executed on Android when the
// running image cannot be. /system/bin requires a writable system partition
// (root or a custom ROM); the app's own native library directory is used when
// the packaging step ships an executable copy there.
func androidFallbackPaths() []string {
	paths := []string{
		"/system/bin/nomad",
		"/system/xbin/nomad",
		"/data/local/tmp/nomad",
	}
	if dir := os.Getenv("NOMAD_ANDROID_LIB_DIR"); dir != "" {
		paths = append([]string{filepath.Join(dir, "libnomad.so")}, paths...)
	}
	return paths
}

// checkExecutable verifies that path exists, is a regular file and carries at
// least one execute bit. It deliberately does not attempt to run the file:
// SELinux denials and noexec mounts would still be possible, but those are
// reported by the re-exec itself with a clearer error.
func checkExecutable(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("%q is a directory", path)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", path)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%q has no execute permission", path)
	}
	return nil
}

// hasOverride reports whether an explicit executable path is configured.
func hasOverride() bool {
	overrideMu.RLock()
	defer overrideMu.RUnlock()
	return override != ""
}

// Self returns the path to the executable of this process.
func Self() string {
	once.Do(func() {
		s, err := executablePath()
		if err != nil {
			panic(fmt.Sprintf("failed to detect executable: %v", err))
		}

		// When running tests, use the real nomad binary. An explicit override
		// is honored as-is: the caller has already chosen the binary, and
		// rewriting it to whatever happens to be first in $PATH would make the
		// override unusable in tests and in embedding hosts.
		if strings.HasSuffix(s, ".test") && !hasOverride() {
			if s, err = exec.LookPath("nomad"); err != nil {
				panic(fmt.Sprintf("failed to find nomad binary: %v", err))
			}
		}
		executable = s
	})
	return executable
}
