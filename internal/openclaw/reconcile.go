package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pvclaw/pvclaw/internal/compiler"
	"github.com/pvclaw/pvclaw/internal/profile"
	"github.com/pvclaw/pvclaw/internal/store"
)

// Reconciler makes the PVClaw profile true in OpenClaw through supported
// interfaces. It only operates on the named profile's agent entry.
type Reconciler struct {
	Client Client
	Store  store.Store
	Now    func() time.Time
}

type ApplyResult struct {
	Diff      Diff
	Created   bool
	Patched   bool
	Recreated bool
	Explain   string
}

// Diff reads the authoritative local profile and compares it with OpenClaw.
func (r Reconciler) Diff(ctx context.Context, agentID string) (Diff, error) {
	desired, _, err := r.desired(agentID)
	if err != nil {
		return Diff{}, err
	}
	actual, err := r.client().GetAgent(ctx, agentID)
	if err != nil {
		return Diff{}, fmt.Errorf("read OpenClaw agent %q: %w", agentID, err)
	}
	return Compare(desired.OpenClaw, actual), nil
}

// Apply reconciles one PVClaw-managed agent. Derived local state is only
// written after the OpenClaw configuration and sandbox verification succeed.
func (r Reconciler) Apply(ctx context.Context, agentID string) (ApplyResult, error) {
	desired, loaded, err := r.desired(agentID)
	if err != nil {
		return ApplyResult{}, err
	}
	actual, err := r.client().GetAgent(ctx, agentID)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("read OpenClaw agent %q: %w", agentID, err)
	}
	initial := Compare(desired.OpenClaw, actual)
	result := ApplyResult{Diff: initial}

	if !actual.Exists {
		if err := r.client().AddAgent(ctx, agentID, desired.PVClaw.WorkspaceDirectory); err != nil {
			return result, fmt.Errorf("create OpenClaw agent %q: %w", agentID, err)
		}
		result.Created = true
		actual, err = r.client().GetAgent(ctx, agentID)
		if err != nil {
			return result, fmt.Errorf("read newly created OpenClaw agent %q: %w", agentID, err)
		}
		if !actual.Exists {
			return result, fmt.Errorf("OpenClaw did not create agent %q", agentID)
		}
	}

	changes := Compare(desired.OpenClaw, actual)
	needsRecreate := !result.Created && changes.NeedsSandboxRecreate()
	if !changes.Empty() {
		if err := r.client().PatchAgent(ctx, desired.OpenClaw, actual.ConfigHash, actual.Tools.Denied); err != nil {
			return result, fmt.Errorf("patch OpenClaw agent %q: %w", agentID, err)
		}
		result.Patched = true

		actual, err = r.client().GetAgent(ctx, agentID)
		if err != nil {
			return result, fmt.Errorf("verify OpenClaw agent %q: %w", agentID, err)
		}
		remaining := Compare(desired.OpenClaw, actual)
		if !remaining.Empty() {
			return result, fmt.Errorf("OpenClaw agent %q remains out of sync:\n%s", agentID, FormatDiff(remaining))
		}
	}

	if needsRecreate {
		if err := r.client().RecreateSandbox(ctx, agentID); err != nil {
			return result, fmt.Errorf("recreate sandbox for %q: %w", agentID, err)
		}
		result.Recreated = true
	}
	explanation, err := r.client().ExplainSandbox(ctx, agentID)
	if err != nil {
		return result, fmt.Errorf("verify sandbox for %q: %w", agentID, err)
	}
	if err := verifySandboxExplanation(desired.OpenClaw, explanation); err != nil {
		return result, fmt.Errorf("sandbox verification for %q: %w", agentID, err)
	}
	result.Explain = explanation

	if err := r.Store.WriteState(loaded.Paths, loaded.Profile, loaded.Raw, r.now()); err != nil {
		return result, fmt.Errorf("write PVClaw derived state: %w", err)
	}
	return result, nil
}

type sandboxExplanation struct {
	AgentID string `json:"agentId"`
	Sandbox struct {
		Mode               string `json:"mode"`
		Scope              string `json:"scope"`
		Backend            string `json:"backend"`
		WorkspaceAccess    string `json:"workspaceAccess"`
		SessionIsSandboxed bool   `json:"sessionIsSandboxed"`
	} `json:"sandbox"`
	Elevated struct {
		Enabled bool `json:"enabled"`
	} `json:"elevated"`
}

func verifySandboxExplanation(desired compiler.DesiredOpenClawState, raw string) error {
	var explanation sandboxExplanation
	if err := json.Unmarshal([]byte(raw), &explanation); err != nil {
		return fmt.Errorf("decode sandbox explain JSON: %w", err)
	}
	if explanation.AgentID != desired.AgentID {
		return fmt.Errorf("agent is %q, want %q", explanation.AgentID, desired.AgentID)
	}
	if explanation.Sandbox.Mode != desired.Sandbox.Mode ||
		explanation.Sandbox.Scope != desired.Sandbox.Scope ||
		explanation.Sandbox.Backend != desired.Sandbox.Backend ||
		explanation.Sandbox.WorkspaceAccess != desired.Sandbox.WorkspaceAccess {
		return fmt.Errorf("effective sandbox is mode=%q backend=%q scope=%q workspaceAccess=%q", explanation.Sandbox.Mode, explanation.Sandbox.Backend, explanation.Sandbox.Scope, explanation.Sandbox.WorkspaceAccess)
	}
	if !explanation.Sandbox.SessionIsSandboxed {
		return fmt.Errorf("effective session is not sandboxed")
	}
	if explanation.Elevated.Enabled {
		return fmt.Errorf("elevated execution remains enabled")
	}
	return nil
}

// desired keeps profile loading, validation, and pure compilation separate
// from OpenClaw mutations.
func (r Reconciler) desired(agentID string) (compiler.DesiredState, reconciliationProfile, error) {
	p, raw, paths, err := r.Store.LoadProfile(agentID)
	if err != nil {
		return compiler.DesiredState{}, reconciliationProfile{}, err
	}
	desired, err := compiler.Compile(p, r.Store.Home)
	if err != nil {
		return compiler.DesiredState{}, reconciliationProfile{}, err
	}
	return desired, reconciliationProfile{Paths: paths, Profile: p, Raw: raw}, nil
}

type reconciliationProfile struct {
	Paths   store.Paths
	Profile profile.Profile
	Raw     []byte
}

func (r Reconciler) client() Client {
	if r.Client == nil {
		return CLIClient{}
	}
	return r.Client
}

func (r Reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}
