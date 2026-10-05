package compiler_test

import (
	"reflect"
	"testing"

	"github.com/pvclaw/pvclaw/internal/compiler"
	"github.com/pvclaw/pvclaw/internal/profile"
)

func TestCompileProfiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		profile      profile.Profile
		network      string
		readOnly     bool
		browser      bool
		expectedDeny []string
	}{
		{
			name: "medical",
			profile: profile.Profile{
				Version:      1,
				Agent:        profile.Agent{ID: "medical", Name: "Medical Agent", Purpose: "medical"},
				Data:         profile.Data{Access: "read-only"},
				Network:      profile.Network{Enabled: false},
				Capabilities: profile.Capabilities{},
				Policy:       profile.Policy{Profile: "medical"},
			},
			network: "none", readOnly: true, browser: false,
			expectedDeny: []string{"browser", "group:web", "exec"},
		},
		{
			name: "work",
			profile: profile.Profile{
				Version:      1,
				Agent:        profile.Agent{ID: "work", Name: "Work Agent", Purpose: "work"},
				Data:         profile.Data{Access: "read-write"},
				Network:      profile.Network{Enabled: true},
				Capabilities: profile.Capabilities{Shell: true, WebSearch: true, Browser: true},
				Policy:       profile.Policy{Profile: "work"},
			},
			network: "bridge", readOnly: false, browser: true,
			expectedDeny: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired, err := compiler.Compile(tt.profile, "/tmp/pvclaw-test")
			if err != nil {
				t.Fatal(err)
			}
			if desired.OpenClaw.Sandbox.Mode != "all" || desired.OpenClaw.Sandbox.Backend != "docker" || desired.OpenClaw.Sandbox.Scope != "agent" || desired.OpenClaw.Sandbox.WorkspaceAccess != "none" {
				t.Fatalf("unexpected sandbox: %#v", desired.OpenClaw.Sandbox)
			}
			if desired.OpenClaw.Sandbox.DockerNetwork != tt.network || desired.OpenClaw.Mount.ReadOnly != tt.readOnly || desired.OpenClaw.Sandbox.BrowserEnabled != tt.browser {
				t.Fatalf("unexpected desired state: %#v", desired.OpenClaw)
			}
			if !desired.OpenClaw.Sandbox.AllowExternalBindSources {
				t.Fatal("PVClaw-managed data bind must authorize its external source")
			}
			if desired.PVClaw.DataDirectory == desired.PVClaw.WorkspaceDirectory || desired.OpenClaw.Workspace != desired.PVClaw.WorkspaceDirectory {
				t.Fatalf("workspace separation = %#v", desired)
			}
			if desired.OpenClaw.Mount.Source != desired.PVClaw.DataDirectory || desired.OpenClaw.Mount.Target != "/data" {
				t.Fatalf("protected bind = %#v", desired.OpenClaw.Mount)
			}
			if !reflect.DeepEqual(desired.OpenClaw.Tools.Denied, tt.expectedDeny) {
				t.Fatalf("denied tools = %#v, want %#v", desired.OpenClaw.Tools.Denied, tt.expectedDeny)
			}
			if desired.OpenClaw.Elevated.Enabled || desired.OpenClaw.ExecHost != "sandbox" {
				t.Fatalf("elevated/exec posture = %#v / %q", desired.OpenClaw.Elevated, desired.OpenClaw.ExecHost)
			}
		})
	}
}

func TestCompileDerivesPathsFromInjectedHome(t *testing.T) {
	t.Parallel()
	p := profile.Profile{
		Version: 1,
		Agent:   profile.Agent{ID: "medical", Name: "Medical", Purpose: "medical"},
		Data:    profile.Data{Access: "read-only"},
		Policy:  profile.Policy{Profile: "medical"},
	}
	desired, err := compiler.Compile(p, "/home/example/.pvclaw")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := desired.PVClaw.AgentDirectory, "/home/example/.pvclaw/agents/medical"; got != want {
		t.Fatalf("agent directory = %q, want %q", got, want)
	}
	if got, want := desired.PVClaw.ProfilePath, "/home/example/.pvclaw/agents/medical/profile.yaml"; got != want {
		t.Fatalf("profile path = %q, want %q", got, want)
	}
	if got, want := desired.PVClaw.DataDirectory, "/home/example/.pvclaw/agents/medical/data"; got != want {
		t.Fatalf("data directory = %q, want %q", got, want)
	}
	if got, want := desired.PVClaw.WorkspaceDirectory, "/home/example/.pvclaw/agents/medical/workspace"; got != want {
		t.Fatalf("workspace directory = %q, want %q", got, want)
	}
	if got, want := desired.PVClaw.StatePath, "/home/example/.pvclaw/agents/medical/state.json"; got != want {
		t.Fatalf("state path = %q, want %q", got, want)
	}
}
