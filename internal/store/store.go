// Package store owns PVClaw-local profile and derived-state persistence.
package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pvclaw/pvclaw/internal/profile"
	"gopkg.in/yaml.v3"
)

const ManagedBy = "pvclaw"

var ErrAgentExists = errors.New("PVClaw-managed agent already exists")

type Paths struct {
	AgentDirectory string
	ProfilePath    string
	DataDirectory  string
	StatePath      string
}

type State struct {
	ManagedBy              string `json:"managed_by"`
	AgentID                string `json:"agent_id"`
	ProfileVersion         int    `json:"profile_version"`
	LastAppliedProfileHash string `json:"last_applied_profile_hash"`
	LastSuccessfulApplyAt  string `json:"last_successful_apply_at,omitempty"`
	OpenClawAgentID        string `json:"openclaw_agent_id"`
}

type Store struct {
	Home string
}

func (s Store) Paths(agentID string) (Paths, error) {
	if !profile.ValidAgentID(agentID) {
		return Paths{}, fmt.Errorf("agent.id: must use lowercase letters, numbers, dashes, or underscores")
	}
	if s.Home == "" {
		return Paths{}, fmt.Errorf("PVClaw home: is required")
	}
	home, err := filepath.Abs(s.Home)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve PVClaw home: %w", err)
	}
	root := filepath.Join(home, "agents")
	agentDirectory := filepath.Join(root, agentID)
	relative, err := filepath.Rel(root, agentDirectory)
	if err != nil || relative == ".." || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return Paths{}, fmt.Errorf("agent.id: resolves outside PVClaw agents directory")
	}
	return Paths{
		AgentDirectory: agentDirectory,
		ProfilePath:    filepath.Join(agentDirectory, "profile.yaml"),
		DataDirectory:  filepath.Join(agentDirectory, "data"),
		StatePath:      filepath.Join(agentDirectory, "state.json"),
	}, nil
}

func (s Store) Create(agentID, template string) (Paths, error) {
	paths, err := s.Paths(agentID)
	if err != nil {
		return Paths{}, err
	}
	if _, err := os.Stat(paths.AgentDirectory); err == nil {
		return Paths{}, fmt.Errorf("%w: %s", ErrAgentExists, agentID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Paths{}, fmt.Errorf("inspect agent directory: %w", err)
	}

	p, err := templateProfile(agentID, template)
	if err != nil {
		return Paths{}, err
	}
	contents, err := yaml.Marshal(p)
	if err != nil {
		return Paths{}, fmt.Errorf("encode profile: %w", err)
	}
	if err := os.MkdirAll(paths.DataDirectory, 0o700); err != nil {
		return Paths{}, fmt.Errorf("create agent data directory: %w", err)
	}
	if err := os.WriteFile(paths.ProfilePath, contents, 0o600); err != nil {
		return Paths{}, fmt.Errorf("write profile: %w", err)
	}
	return paths, nil
}

func (s Store) LoadProfile(agentID string) (profile.Profile, []byte, Paths, error) {
	paths, err := s.Paths(agentID)
	if err != nil {
		return profile.Profile{}, nil, Paths{}, err
	}
	contents, err := os.ReadFile(paths.ProfilePath)
	if err != nil {
		return profile.Profile{}, nil, Paths{}, fmt.Errorf("read profile: %w", err)
	}
	p, err := profile.Parse(contents)
	if err != nil {
		return profile.Profile{}, nil, Paths{}, err
	}
	if err := profile.Validate(p); err != nil {
		return profile.Profile{}, nil, Paths{}, err
	}
	if p.Agent.ID != agentID {
		return profile.Profile{}, nil, Paths{}, fmt.Errorf("agent.id: profile ID %q does not match requested agent %q", p.Agent.ID, agentID)
	}
	return p, contents, paths, nil
}

func (s Store) WriteState(paths Paths, p profile.Profile, profileContents []byte, now time.Time) error {
	digest := sha256.Sum256(profileContents)
	state := State{
		ManagedBy:              ManagedBy,
		AgentID:                p.Agent.ID,
		ProfileVersion:         p.Version,
		LastAppliedProfileHash: fmt.Sprintf("%x", digest),
		OpenClawAgentID:        p.Agent.ID,
	}
	if !now.IsZero() {
		state.LastSuccessfulApplyAt = now.UTC().Format(time.RFC3339)
	}
	contents, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode derived state: %w", err)
	}
	contents = append(contents, '\n')
	if err := writeFileAtomically(paths.StatePath, contents, 0o600); err != nil {
		return fmt.Errorf("write derived state: %w", err)
	}
	return nil
}

func templateProfile(agentID, template string) (profile.Profile, error) {
	switch template {
	case "medical":
		return profile.Profile{
			Version: 1,
			Agent:   profile.Agent{ID: agentID, Name: "Medical Agent", Purpose: "medical"},
			Data:    profile.Data{Access: "read-only"},
			Network: profile.Network{Enabled: false},
			Capabilities: profile.Capabilities{
				Shell: false, WebSearch: false, Browser: false, SimulatedMessaging: false,
			},
			Policy: profile.Policy{Profile: "medical"},
		}, nil
	case "work":
		return profile.Profile{
			Version: 1,
			Agent:   profile.Agent{ID: agentID, Name: "Work Agent", Purpose: "work"},
			Data:    profile.Data{Access: "read-write"},
			Network: profile.Network{Enabled: true},
			Capabilities: profile.Capabilities{
				Shell: true, WebSearch: true, Browser: true, SimulatedMessaging: false,
			},
			Policy: profile.Policy{Profile: "work"},
		}, nil
	default:
		return profile.Profile{}, fmt.Errorf("template: must be medical or work")
	}
}

func writeFileAtomically(path string, contents []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, ".pvclaw-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(contents); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
