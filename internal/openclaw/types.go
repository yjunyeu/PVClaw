// Package openclaw contains the supported-interface adapter and reconciliation logic.
package openclaw

import (
	"context"

	"github.com/pvclaw/pvclaw/internal/compiler"
)

type ActualAgentState struct {
	Exists     bool
	AgentID    string
	ConfigHash string
	Workspace  string
	Sandbox    ActualSandboxState
	Mount      ActualBindMount
	Tools      ActualToolState
}

type ActualSandboxState struct {
	Mode                     string
	Backend                  string
	Scope                    string
	WorkspaceAccess          string
	DockerNetwork            string
	AllowExternalBindSources bool
	BrowserEnabled           bool
}

type ActualBindMount struct {
	Found    bool
	Source   string
	Target   string
	ReadOnly bool
}

type ActualToolState struct {
	Denied        []string
	BrowserDenied bool
	WebDenied     bool
	ShellDenied   bool
	ElevatedOn    bool
	ExecHost      string
}

// Client provides only the supported OpenClaw operations PVClaw needs.
type Client interface {
	GetAgent(context.Context, string) (ActualAgentState, error)
	AddAgent(context.Context, string, string) error
	PatchAgent(context.Context, compiler.DesiredOpenClawState, string, []string) error
	RecreateSandbox(context.Context, string) error
	ExplainSandbox(context.Context, string) (string, error)
}

type ChangeKind string

const (
	ChangeCreate        ChangeKind = "create"
	ChangeRestrictive   ChangeKind = "restrictive"
	ChangeBroadening    ChangeKind = "broadening"
	ChangeConfiguration ChangeKind = "configuration"
)

type Change struct {
	Field   string
	Desired string
	Actual  string
	Kind    ChangeKind
}

type Diff struct {
	AgentID string
	Changes []Change
}

func (d Diff) Empty() bool {
	return len(d.Changes) == 0
}

func (d Diff) NeedsCreate() bool {
	for _, change := range d.Changes {
		if change.Kind == ChangeCreate {
			return true
		}
	}
	return false
}

func (d Diff) NeedsSandboxRecreate() bool {
	for _, change := range d.Changes {
		switch change.Field {
		case "sandbox.mode", "sandbox.backend", "sandbox.scope", "sandbox.workspace_access", "sandbox.docker.network", "sandbox.docker.dangerously_allow_external_bind_sources", "sandbox.browser.enabled", "mount./data":
			return true
		}
	}
	return false
}
