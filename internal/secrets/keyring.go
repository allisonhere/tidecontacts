// Package secrets stores short-lived credentials (currently just a Google
// app password) in the OS keychain rather than in TideContacts' SQLite
// database or any config file. The backend and probe logic mirror TideMail's
// internal/config/keyring.go, which is the only proven keychain integration
// in the Tide family — ported here rather than re-derived.
package secrets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const appName = "tidecontacts"

// opTimeout bounds every keychain CLI invocation. A locked collection with an
// activatable-but-headless prompter (e.g. org.kde.secretprompter over SSH)
// can otherwise hang the call — and startup with it.
const opTimeout = 3 * time.Second

type backend struct {
	tool       string
	storeArgs  func(key string) []string
	stdin      bool // secret-tool reads the value from stdin; security takes it as an argv
	lookupArgs func(key string) []string
	clearArgs  func(key string) []string
}

func backendFor(goos string) (b backend, ok bool) {
	switch goos {
	case "linux":
		return backend{
			tool: "secret-tool",
			storeArgs: func(key string) []string {
				return []string{"store", "--label=" + appName, "application", appName, "key", key}
			},
			stdin: true,
			lookupArgs: func(key string) []string {
				return []string{"lookup", "application", appName, "key", key}
			},
			clearArgs: func(key string) []string {
				return []string{"clear", "application", appName, "key", key}
			},
		}, true
	case "darwin":
		return backend{
			tool: "security",
			storeArgs: func(key string) []string {
				return []string{"add-generic-password", "-U", "-a", key, "-s", appName, "-w"}
			},
			stdin: false,
			lookupArgs: func(key string) []string {
				return []string{"find-generic-password", "-a", key, "-s", appName, "-w"}
			},
			clearArgs: func(key string) []string {
				return []string{"delete-generic-password", "-a", key, "-s", appName}
			},
		}, true
	default:
		return backend{}, false
	}
}

func installedFor(goos string) bool {
	b, ok := backendFor(goos)
	if !ok {
		return false
	}
	_, err := exec.LookPath(b.tool)
	return err == nil
}

var (
	probeOnce   sync.Once
	probeResult bool
	probeReason string
)

// disabled reads the TIDECONTACTS_DISABLE_KEYRING opt-out on every call
// rather than caching it, so a test can scope it to itself. The keychain is
// per-user and global: a test using real account/path names would otherwise
// read and overwrite live secrets.
func disabled() bool {
	v := strings.TrimSpace(os.Getenv("TIDECONTACTS_DISABLE_KEYRING"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// Usable reports whether the system keychain can actually store and retrieve
// a secret right now, probing once per process with a sentinel round-trip. A
// locked collection, a missing session bus, or a hung prompter all fail the
// probe, and the caller falls back to telling the user rather than silently
// losing the secret.
func Usable() (ok bool, reason string) { return usableFor(runtime.GOOS) }

func usableFor(goos string) (bool, string) {
	if disabled() {
		return false, "credential storage disabled by TIDECONTACTS_DISABLE_KEYRING"
	}
	probeOnce.Do(func() {
		if !installedFor(goos) {
			probeReason = "the system keychain tool is not installed"
			return
		}
		const k = "selftest"
		want := "ok-" + time.Now().Format("150405.000")
		if err := storeFor(goos, k, want); err != nil {
			probeReason = "keychain is locked or unreachable (store failed)"
			return
		}
		got := lookupFor(goos, k)
		_ = clearFor(goos, k)
		if got != want {
			probeReason = "keychain is locked or unreachable (readback failed)"
			return
		}
		probeResult = true
	})
	return probeResult, probeReason
}

func runTool(tool string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, tool, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", tool, strings.Join(args, " "), err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func storeFor(goos, key, value string) error {
	if value == "" {
		return nil
	}
	b, ok := backendFor(goos)
	if !ok {
		return fmt.Errorf("no keychain backend for %s", goos)
	}
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()
	args := b.storeArgs(key)
	if !b.stdin {
		args = append(args, value)
	}
	cmd := exec.CommandContext(ctx, b.tool, args...)
	if b.stdin {
		cmd.Stdin = strings.NewReader(value + "\n")
	}
	return cmd.Run()
}

func lookupFor(goos, key string) string {
	b, ok := backendFor(goos)
	if !ok {
		return ""
	}
	out, err := runTool(b.tool, b.lookupArgs(key)...)
	if err != nil {
		return ""
	}
	return out
}

func clearFor(goos, key string) error {
	b, ok := backendFor(goos)
	if !ok {
		return fmt.Errorf("no keychain backend for %s", goos)
	}
	_, err := runTool(b.tool, b.clearArgs(key)...)
	return err
}

// Store saves a secret in the system keychain under key. It reports false if
// the keychain is unavailable or the value could not be written.
func Store(key, value string) bool {
	ok, _ := Usable()
	if !ok || key == "" || value == "" {
		return false
	}
	return storeFor(runtime.GOOS, key, value) == nil
}

// Lookup retrieves a secret from the system keychain, or "" if it is absent
// or the keychain is unavailable.
func Lookup(key string) string {
	ok, _ := Usable()
	if !ok {
		return ""
	}
	return lookupFor(runtime.GOOS, key)
}

// Clear removes a secret from the system keychain. It is a no-op, not an
// error, when the keychain is unavailable or the key was never stored.
func Clear(key string) {
	ok, _ := Usable()
	if !ok {
		return
	}
	_ = clearFor(runtime.GOOS, key)
}
