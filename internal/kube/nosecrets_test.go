package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tool's central promise is that it never reads Secret values. This test
// fails the build if a Secrets API call is ever added, which is stronger than a
// comment or a code review.
func TestNoSecretsAPICall(t *testing.T) {
	forbidden := []string{"Secrets(", "SecretList"}

	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
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
