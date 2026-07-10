// SPDX-License-Identifier: AGPL-3.0-only

package ops_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// The release image sets GOTOOLCHAIN=local, so its pinned builder must satisfy
// the go.mod directive. Otherwise local CI passes while deployment fails during
// `go mod download` before the binary is built.
func TestDockerBuilderMatchesGoDirective(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	gomod := mustRead(t, filepath.Join(root, "go.mod"))
	dockerfile := mustRead(t, filepath.Join(root, "Dockerfile"))

	goMatch := regexp.MustCompile(`(?m)^go ([0-9]+\.[0-9]+\.[0-9]+)$`).FindStringSubmatch(gomod)
	if len(goMatch) != 2 {
		t.Fatal("go.mod must carry a full patch-level Go directive")
	}
	builderMatch := regexp.MustCompile(`(?m)^FROM golang:([0-9]+\.[0-9]+\.[0-9]+)-`).FindStringSubmatch(dockerfile)
	if len(builderMatch) != 2 {
		t.Fatal("Dockerfile must pin a patch-level golang builder tag")
	}
	if builderMatch[1] != goMatch[1] {
		t.Fatalf("Docker builder Go %s does not match go.mod %s", builderMatch[1], goMatch[1])
	}
	if !strings.Contains(dockerfile, "golang:"+goMatch[1]+"-bookworm@sha256:") {
		t.Fatal("Docker builder must remain digest-pinned")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
