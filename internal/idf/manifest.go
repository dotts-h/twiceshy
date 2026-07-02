package idf

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ManifestSource is one data source entry in a Manifest.
type ManifestSource struct {
	Name    string `yaml:"name"`
	Path    string `yaml:"path"`
	License string `yaml:"license"`
}

// Manifest is the parsed form of a YAML manifest file listing data sources.
type Manifest struct {
	Sources []ManifestSource `yaml:"sources"`
}

// loadManifest reads the YAML manifest file at path and parses it into a
// Manifest. It returns a nil *Manifest alongside a non-nil error if the file
// cannot be read or does not contain valid YAML.
func loadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}
