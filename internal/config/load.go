package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Parse decodes policy YAML from bytes into a typed Policy.
func Parse(data []byte) (*Policy, error) {
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse policy yaml: %w", err)
	}
	return &p, nil
}

// Load reads and parses a policy YAML file from disk.
func Load(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy file %s: %w", path, err)
	}
	return Parse(data)
}
