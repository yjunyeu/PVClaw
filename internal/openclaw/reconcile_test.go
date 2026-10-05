package openclaw

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/pvclaw/pvclaw/internal/compiler"
	"github.com/pvclaw/pvclaw/internal/store"
)

type fakeClient struct {
	actual          ActualAgentState
	addCalls        int
	addWorkspace    string
	patchCalls      int
	recreateCalls   int
	explainCalls    int
	patchError      error
	explainError    error
	unrelatedExists bool
}

func (f *fakeClient) GetAgent(_ context.Context, _ string) (ActualAgentState, error) {
	return f.actual, nil
}

func (f *fakeClient) AddAgent(_ context.Context, agentID, workspace string) error {
	f.addCalls++
	f.addWorkspace = workspace
	f.actual = ActualAgentState{Exists: true, AgentID: agentID, ConfigHash: "created-hash"}
	return nil
}

func (f *fakeClient) PatchAgent(_ context.Context, desired compiler.DesiredOpenClawState, _ string, _ []string) error {
	f.patchCalls++
	if f.patchError != nil {
		return f.patchError
	}
	f.actual = actualFor(desired)
	return nil
}

func (f *fakeClient) RecreateSandbox(_ context.Context, _ string) error {
	f.recreateCalls++
	return nil
}

func (f *fakeClient) ExplainSandbox(_ context.Context, _ string) (string, error) {
	f.explainCalls++
	if f.explainError != nil {
		return "", f.explainError
	}
	return `{"agentId":"medical","sandbox":{"mode":"all","scope":"agent","backend":"docker","workspaceAccess":"none","sessionIsSandboxed":true},"elevated":{"enabled":false}}`, nil
}

func medicalReconciler(t *testing.T, actual ActualAgentState) (Reconciler, store.Paths) {
	t.Helper()
	s := store.Store{Home: t.TempDir()}
	paths, err := s.Create("medical", "medical")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	client := &fakeClient{actual: actual, unrelatedExists: true}
	return Reconciler{
		Client: client,
		Store:  s,
		Now:    func() time.Time { return time.Date(2026, 10, 3, 1, 2, 3, 0, time.UTC) },
	}, paths
}

func desiredMedical(t *testing.T, reconciler Reconciler) compiler.DesiredOpenClawState {
	t.Helper()
	p, _, _, err := reconciler.Store.LoadProfile("medical")
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	desired, err := compiler.Compile(p, reconciler.Store.Home)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return desired.OpenClaw
}

func actualFor(desired compiler.DesiredOpenClawState) ActualAgentState {
	return ActualAgentState{
		Exists: true, AgentID: desired.AgentID, ConfigHash: "hash",
		Workspace: desired.Workspace,
		Sandbox: ActualSandboxState{
			Mode: desired.Sandbox.Mode, Backend: desired.Sandbox.Backend, Scope: desired.Sandbox.Scope,
			WorkspaceAccess: desired.Sandbox.WorkspaceAccess, DockerNetwork: desired.Sandbox.DockerNetwork,
			AllowExternalBindSources: desired.Sandbox.AllowExternalBindSources,
			BrowserEnabled:           desired.Sandbox.BrowserEnabled,
		},
		Mount: ActualBindMount{Found: true, Source: desired.Mount.Source, Target: desired.Mount.Target, ReadOnly: desired.Mount.ReadOnly},
		Tools: ActualToolState{
			Denied:        append([]string(nil), desired.Tools.Denied...),
			BrowserDenied: contains(desired.Tools.Denied, "browser"),
			WebDenied:     contains(desired.Tools.Denied, "group:web"),
			ShellDenied:   contains(desired.Tools.Denied, "exec"),
			ElevatedOn:    false, ExecHost: desired.ExecHost,
		},
	}
}

func TestDiffMedicalMissingRequiresCreate(t *testing.T) {
	reconciler, _ := medicalReconciler(t, ActualAgentState{AgentID: "medical"})
	diff, err := reconciler.Diff(context.Background(), "medical")
	if err != nil || !diff.NeedsCreate() {
		t.Fatalf("Diff() = %#v, %v; want create", diff, err)
	}
}

func TestDiffMedicalSynchronized(t *testing.T) {
	reconciler, _ := medicalReconciler(t, ActualAgentState{})
	reconciler.Client.(*fakeClient).actual = actualFor(desiredMedical(t, reconciler))
	diff, err := reconciler.Diff(context.Background(), "medical")
	if err != nil || !diff.Empty() {
		t.Fatalf("Diff() = %#v, %v; want empty", diff, err)
	}
}

func TestDiffCallsOutRestrictiveChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ActualAgentState)
		field  string
	}{
		{"network", func(a *ActualAgentState) { a.Sandbox.DockerNetwork = "bridge" }, "sandbox.docker.network"},
		{"mount", func(a *ActualAgentState) { a.Mount.ReadOnly = false }, "mount./data"},
		{"browser", func(a *ActualAgentState) { a.Sandbox.BrowserEnabled = true }, "sandbox.browser.enabled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reconciler, _ := medicalReconciler(t, ActualAgentState{})
			actual := actualFor(desiredMedical(t, reconciler))
			test.mutate(&actual)
			reconciler.Client.(*fakeClient).actual = actual
			diff, err := reconciler.Diff(context.Background(), "medical")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, change := range diff.Changes {
				if change.Field == test.field && change.Kind == ChangeRestrictive {
					found = true
				}
			}
			if !found {
				t.Fatalf("diff did not call out restrictive %s: %#v", test.field, diff)
			}
		})
	}
}

func TestApplyCreatesMissingAgentAndWritesStateAfterSuccess(t *testing.T) {
	reconciler, paths := medicalReconciler(t, ActualAgentState{AgentID: "medical"})
	client := reconciler.Client.(*fakeClient)
	result, err := reconciler.Apply(context.Background(), "medical")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || !result.Patched || result.Recreated || client.addCalls != 1 || client.patchCalls != 1 {
		t.Fatalf("unexpected apply result %#v, client %#v", result, client)
	}
	if got, want := client.addWorkspace, paths.WorkspaceDirectory; got != want {
		t.Fatalf("AddAgent workspace = %q, want %q", got, want)
	}
	if client.addWorkspace == paths.DataDirectory {
		t.Fatal("AddAgent used protected data directory as its workspace")
	}
	if !client.unrelatedExists {
		t.Fatal("unrelated OpenClaw agent was changed")
	}
	if _, err := os.Stat(paths.StatePath); err != nil {
		t.Fatalf("state file was not written after success: %v", err)
	}
}

func TestDiffReportsWorkspaceDrift(t *testing.T) {
	reconciler, _ := medicalReconciler(t, ActualAgentState{})
	client := reconciler.Client.(*fakeClient)
	client.actual = actualFor(desiredMedical(t, reconciler))
	client.actual.Workspace = "/wrong/workspace"
	diff, err := reconciler.Diff(context.Background(), "medical")
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Changes) != 1 || diff.Changes[0].Field != "workspace" {
		t.Fatalf("workspace drift diff = %#v", diff)
	}
}

func TestDiffReportsExternalBindSourceAuthorizationDrift(t *testing.T) {
	reconciler, _ := medicalReconciler(t, ActualAgentState{})
	client := reconciler.Client.(*fakeClient)
	client.actual = actualFor(desiredMedical(t, reconciler))
	client.actual.Sandbox.AllowExternalBindSources = false
	diff, err := reconciler.Diff(context.Background(), "medical")
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Changes) != 1 || diff.Changes[0].Field != "sandbox.docker.dangerously_allow_external_bind_sources" || diff.Changes[0].Desired != "enabled" || diff.Changes[0].Actual != "disabled" {
		t.Fatalf("external bind source drift = %#v", diff)
	}
	if !diff.NeedsSandboxRecreate() {
		t.Fatal("external bind source authorization should require sandbox recreation")
	}
}

func TestApplyReconcilesIncorrectWorkspace(t *testing.T) {
	reconciler, _ := medicalReconciler(t, ActualAgentState{})
	client := reconciler.Client.(*fakeClient)
	desired := desiredMedical(t, reconciler)
	client.actual = actualFor(desired)
	client.actual.Workspace = "/wrong/workspace"
	result, err := reconciler.Apply(context.Background(), "medical")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Patched || result.Recreated || client.actual.Workspace != desired.Workspace {
		t.Fatalf("workspace reconciliation result = %#v, actual = %#v", result, client.actual)
	}
}

func TestApplyDoesNotRecreateSynchronizedSandbox(t *testing.T) {
	reconciler, _ := medicalReconciler(t, ActualAgentState{})
	client := reconciler.Client.(*fakeClient)
	client.actual = actualFor(desiredMedical(t, reconciler))
	result, err := reconciler.Apply(context.Background(), "medical")
	if err != nil {
		t.Fatal(err)
	}
	if result.Patched || result.Recreated || client.recreateCalls != 0 {
		t.Fatalf("synchronized apply recreated or patched: %#v", result)
	}
}

func TestApplyRecreatesSandboxForNetworkOrBindChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ActualAgentState)
	}{
		{"network", func(a *ActualAgentState) { a.Sandbox.DockerNetwork = "bridge" }},
		{"bind", func(a *ActualAgentState) { a.Mount.ReadOnly = false }},
		{"external bind sources", func(a *ActualAgentState) { a.Sandbox.AllowExternalBindSources = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reconciler, _ := medicalReconciler(t, ActualAgentState{})
			client := reconciler.Client.(*fakeClient)
			client.actual = actualFor(desiredMedical(t, reconciler))
			test.mutate(&client.actual)
			result, err := reconciler.Apply(context.Background(), "medical")
			if err != nil {
				t.Fatal(err)
			}
			if !result.Recreated || client.recreateCalls != 1 {
				t.Fatalf("expected sandbox recreation, result %#v", result)
			}
		})
	}
}

func TestFailedApplyDoesNotWriteState(t *testing.T) {
	reconciler, paths := medicalReconciler(t, ActualAgentState{})
	client := reconciler.Client.(*fakeClient)
	client.actual = actualFor(desiredMedical(t, reconciler))
	client.actual.Sandbox.DockerNetwork = "bridge"
	client.patchError = errors.New("gateway rejected patch")
	if _, err := reconciler.Apply(context.Background(), "medical"); err == nil {
		t.Fatal("Apply() unexpectedly succeeded")
	}
	if _, err := os.Stat(paths.StatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file was written after failed apply: %v", err)
	}
}
