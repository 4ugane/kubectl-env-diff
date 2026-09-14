package kube

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The tool's central promise is that it never reads Secret values. This test
// fails the build if a Secrets API call is ever added, which is stronger than a
// comment or a code review.
func TestNoSecretsAPICall(t *testing.T) {
	forbidden := []string{"Secrets(", "SecretList"}

	root := moduleRoot(t)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, f := range forbidden {
			if strings.Contains(string(data), f) {
				t.Errorf("%s references %q; the tool must never call the Secrets API", path, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking source tree: %v", err)
	}
}

// moduleRoot finds the repository root (the directory containing go.mod) by
// walking up from this test file's own location. Walking from the module
// root, rather than a fixed ".." relative to internal/kube, ensures the
// no-Secrets-API guard covers the whole tree - including cmd/, the actual
// binary entrypoint - regardless of which package the test lives in.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine this test file's path")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above " + thisFile)
		}
		dir = parent
	}
}
