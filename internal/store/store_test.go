package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateScaffoldsMedicalAgent(t *testing.T) {
	root := t.TempDir()
	s := Store{Home: root}
	paths, err := s.Create("medical", "medical")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if paths.DataDirectory != filepath.Join(root, "agents", "medical", "data") {
		t.Fatalf("data directory = %q", paths.DataDirectory)
	}
	if info, err := os.Stat(paths.DataDirectory); err != nil || !info.IsDir() {
		t.Fatalf("data directory missing or not directory: %v", err)
	}
	p, _, _, err := s.LoadProfile("medical")
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if p.Agent.ID != "medical" || p.Agent.Purpose != "medical" || p.Data.Access != "read-only" || p.Network.Enabled {
		t.Fatalf("unexpected medical template: %#v", p)
	}
	if _, err := os.Stat(paths.StatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file should not exist before apply, stat error = %v", err)
	}
}

func TestCreateRefusesExistingAgent(t *testing.T) {
	s := Store{Home: t.TempDir()}
	if _, err := s.Create("medical", "medical"); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if _, err := s.Create("medical", "medical"); !errors.Is(err, ErrAgentExists) {
		t.Fatalf("second Create() error = %v, want ErrAgentExists", err)
	}
}
