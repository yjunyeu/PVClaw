// Package compiler converts validated PVClaw privacy intent into desired state.
package compiler

import (
	"fmt"
	"path/filepath"

	"github.com/pvclaw/pvclaw/internal/profile"
)

type DesiredState struct {
	PVClaw      PVClawState
	OpenClaw    DesiredOpenClawState
	DefenseClaw DesiredDefenseClawState
}

type PVClawState struct {
	AgentDirectory     string
	ProfilePath        string
	DataDirectory      string
	WorkspaceDirectory string
	StatePath          string
}

type DesiredOpenClawState struct {
	AgentID   string
	AgentName string
	Purpose   string
	Workspace string
	Sandbox   SandboxState
	Mount     BindMount
	Tools     ToolPolicy
	Elevated  ElevatedExecution
	ExecHost  string
}

type SandboxState struct {
	Mode                     string
	Backend                  string
	Scope                    string
	WorkspaceAccess          string
	DockerNetwork            string
	AllowExternalBindSources bool
	BrowserEnabled           bool
}

type BindMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type ToolPolicy struct {
	Denied []string
}

type ElevatedExecution struct {
	Enabled bool
}

type DesiredDefenseClawState struct {
	BasePolicy    string
	PolicyProfile string
	AgentID       string
}

func Compile(p profile.Profile, home string) (DesiredState, error) {
	if err := profile.Validate(p); err != nil {
		return DesiredState{}, err
	}
	if home == "" {
		return DesiredState{}, fmt.Errorf("PVClaw home: is required")
	}

	paths := derivePaths(home, p.Agent.ID)
	network := "bridge"
	if !p.Network.Enabled {
		network = "none"
	}

	return DesiredState{
		PVClaw: paths,
		OpenClaw: DesiredOpenClawState{
			AgentID:   p.Agent.ID,
			AgentName: p.Agent.Name,
			Purpose:   p.Agent.Purpose,
			Workspace: paths.WorkspaceDirectory,
			Sandbox: SandboxState{
				Mode:            "all",
				Backend:         "docker",
				Scope:           "agent",
				WorkspaceAccess: "none",
				DockerNetwork:   network,
				// PVClaw derives the bind source under its own managed agent root;
				// profiles never provide arbitrary host paths.
				AllowExternalBindSources: true,
				BrowserEnabled:           p.Capabilities.Browser,
			},
			Mount: BindMount{
				Source:   paths.DataDirectory,
				Target:   "/data",
				ReadOnly: p.Data.Access == "read-only",
			},
			Tools:    ToolPolicy{Denied: deniedTools(p.Capabilities)},
			Elevated: ElevatedExecution{Enabled: false},
			ExecHost: "sandbox",
		},
		DefenseClaw: DesiredDefenseClawState{
			BasePolicy:    "default",
			PolicyProfile: p.Policy.Profile,
			AgentID:       p.Agent.ID,
		},
	}, nil
}

func derivePaths(home, agentID string) PVClawState {
	agentDirectory := filepath.Join(filepath.Clean(home), "agents", agentID)
	return PVClawState{
		AgentDirectory:     agentDirectory,
		ProfilePath:        filepath.Join(agentDirectory, "profile.yaml"),
		DataDirectory:      filepath.Join(agentDirectory, "data"),
		WorkspaceDirectory: filepath.Join(agentDirectory, "workspace"),
		StatePath:          filepath.Join(agentDirectory, "state.json"),
	}
}

func deniedTools(capabilities profile.Capabilities) []string {
	denied := make([]string, 0, 3)
	if !capabilities.Browser {
		denied = append(denied, "browser")
	}
	if !capabilities.WebSearch {
		denied = append(denied, "group:web")
	}
	if !capabilities.Shell {
		denied = append(denied, "exec")
	}
	return denied
}
