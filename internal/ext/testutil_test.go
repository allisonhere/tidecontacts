package ext

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type fixture struct {
	t                   *testing.T
	root, dir, data, st string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script extensions are Unix only")
	}
	root := t.TempDir()
	return &fixture{t: t, root: root, dir: filepath.Join(root, "extensions"), data: filepath.Join(root, "data"), st: filepath.Join(root, "state.json")}
}

// add writes extension id with the manifest body (after the required id/name lines) and an sh script.
func (f *fixture) add(id, manifest, script string) string {
	f.t.Helper()
	dir := filepath.Join(f.dir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	head := "id = \"" + id + "\"\nname = \"Test " + id + "\"\nversion = \"1.0.0\"\napi = 1\ncommand = \"run.sh\"\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(head+manifest), 0o644); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		f.t.Fatal(err)
	}
	return dir
}

func (f *fixture) manager(o Options) *Manager {
	f.t.Helper()
	o.Dir, o.DataDir, o.StateFile = f.dir, f.data, f.st
	if o.Timeout == 0 {
		o.Timeout = 2 * time.Second
	}
	m := New(o)
	if err := m.Discover(context.Background()); err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(m.Close)
	return m
}

// okScript answers every request with ok and the given data, reading one line first.
func okScript(data string) string {
	return "read line\nid=$(printf '%s' \"$line\" | sed 's/.*\"request_id\":\"\\([^\"]*\\)\".*/\\1/')\nprintf '{\"api\":1,\"type\":\"response\",\"request_id\":\"%s\",\"ok\":true,\"data\":%s}\\n' \"$id\" '" + data + "'\n"
}
