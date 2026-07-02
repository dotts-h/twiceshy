package idf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadManifest_ParsesFixtureSources verifies loadManifest reads a YAML
// manifest file into a *Manifest whose Sources slice preserves each entry's
// Name, Path, and License fields in file order.
func TestLoadManifest_ParsesFixtureSources(t *testing.T) {
	fixture := `sources:
  - name: nvd
    path: /data/nvd
    license: public-domain
  - name: osv
    path: /data/osv
    license: Apache-2.0
`
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("WriteFile(fixture) returned error: %v", err)
	}

	manifest, err := loadManifest(manifestPath)
	if err != nil {
		t.Fatalf("loadManifest(%q) returned error: %v", manifestPath, err)
	}

	if got := len(manifest.Sources); got != 2 {
		t.Fatalf("len(manifest.Sources) = %d, want 2", got)
	}

	first := manifest.Sources[0]
	if first.Name != "nvd" {
		t.Fatalf("Sources[0].Name = %q, want %q", first.Name, "nvd")
	}
	if first.Path != "/data/nvd" {
		t.Fatalf("Sources[0].Path = %q, want %q", first.Path, "/data/nvd")
	}
	if first.License != "public-domain" {
		t.Fatalf("Sources[0].License = %q, want %q", first.License, "public-domain")
	}

	second := manifest.Sources[1]
	if second.Name != "osv" {
		t.Fatalf("Sources[1].Name = %q, want %q", second.Name, "osv")
	}
	if second.Path != "/data/osv" {
		t.Fatalf("Sources[1].Path = %q, want %q", second.Path, "/data/osv")
	}
	if second.License != "Apache-2.0" {
		t.Fatalf("Sources[1].License = %q, want %q", second.License, "Apache-2.0")
	}
}

// TestLoadManifest_MissingFileErrors verifies loadManifest surfaces an error
// (rather than a zero-value *Manifest with nil error) when the path does not
// exist on disk.
func TestLoadManifest_MissingFileErrors(t *testing.T) {
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "does-not-exist.yaml")

	manifest, err := loadManifest(missingPath)
	if err == nil {
		t.Fatalf("loadManifest(%q) returned nil error for missing file, manifest = %+v", missingPath, manifest)
	}
	if manifest != nil {
		t.Fatalf("loadManifest(%q) returned non-nil manifest %+v alongside error %v", missingPath, manifest, err)
	}
}

// TestLoadManifest_MalformedYAMLErrors verifies loadManifest surfaces an
// error when the file exists but contains YAML that cannot be unmarshaled
// into a Manifest (here, a scalar where a mapping is expected).
func TestLoadManifest_MalformedYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	malformedPath := filepath.Join(dir, "malformed.yaml")
	if err := os.WriteFile(malformedPath, []byte("sources: \"not a list\"\n:::not-yaml:::\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(malformed) returned error: %v", err)
	}

	manifest, err := loadManifest(malformedPath)
	if err == nil {
		t.Fatalf("loadManifest(%q) returned nil error for malformed YAML, manifest = %+v", malformedPath, manifest)
	}
	if manifest != nil {
		t.Fatalf("loadManifest(%q) returned non-nil manifest %+v alongside error %v", malformedPath, manifest, err)
	}
}
