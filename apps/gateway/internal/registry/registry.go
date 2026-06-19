package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Tool struct {
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description" yaml:"description"`
	Parameters  map[string]any `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Connector   string         `json:"connector" yaml:"-"`
}

type manifest struct {
	Connector   string `yaml:"connector"`
	DisplayName string `yaml:"display_name"`
	Tools       []Tool `yaml:"tools"`
}

type Registry struct {
	tools []Tool
}

func LoadFromDir(connectorsDir string) (*Registry, error) {
	entries, err := os.ReadDir(connectorsDir)
	if err != nil {
		return nil, fmt.Errorf("read connectors dir: %w", err)
	}

	var tools []Tool
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(connectorsDir, entry.Name(), "manifest.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var m manifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, t := range m.Tools {
			t.Connector = m.Connector
			tools = append(tools, t)
		}
	}

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	return &Registry{tools: tools}, nil
}

func (r *Registry) List() []Tool {
	out := make([]Tool, len(r.tools))
	copy(out, r.tools)
	return out
}

func (r *Registry) Get(name string) (Tool, bool) {
	for _, t := range r.tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func (r *Registry) Search(query string) []Tool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return r.List()
	}
	var out []Tool
	for _, t := range r.tools {
		if strings.Contains(strings.ToLower(t.Name), q) ||
			strings.Contains(strings.ToLower(t.Description), q) ||
			strings.Contains(strings.ToLower(t.Connector), q) {
			out = append(out, t)
		}
	}
	return out
}
