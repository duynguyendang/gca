package ingest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ComponentMetadata defines metadata for a specific component within the project.
type ComponentMetadata struct {
	Type     string `yaml:"type"`
	Language string `yaml:"language"`
	Path     string `yaml:"path"`
}

// ProjectMetadata defines the structure of the project.yaml file.
type ProjectMetadata struct {
	Name        string                       `yaml:"name"`
	Description string                       `yaml:"description"`
	Version     string                       `yaml:"version"`
	Tags        []string                     `yaml:"tags"`
	Components  map[string]ComponentMetadata `yaml:"components"`
}

// LoadProjectMetadata reads and parses the project.yaml file from the given path.
func LoadProjectMetadata(path string) (*ProjectMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read project metadata: %w", err)
	}

	var metadata ProjectMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse project metadata: %w", err)
	}

	return &metadata, nil
}
