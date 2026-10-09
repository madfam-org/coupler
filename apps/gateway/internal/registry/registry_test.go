package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDir(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "connectors")
	if _, err := os.Stat(root); err != nil {
		t.Skip("connectors dir not found from test cwd")
	}
	reg, err := LoadFromDir(root)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	tools := reg.List()
	if len(tools) < 4 {
		t.Fatalf("expected at least 4 tools, got %d", len(tools))
	}
	if _, ok := reg.Get("coupler.github.list_repos"); !ok {
		t.Fatal("missing coupler.github.list_repos")
	}
	found := reg.Search("slack")
	if len(found) < 2 {
		t.Fatalf("expected slack tools, got %d", len(found))
	}
}
