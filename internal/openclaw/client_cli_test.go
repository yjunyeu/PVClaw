package openclaw

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pvclaw/pvclaw/internal/compiler"
)

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	output []byte
	calls  []recordedCommand
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, recordedCommand{name: name, args: args})
	return r.output, nil
}

func TestCLIClientUsesGatewayGetAndSparseAgentPatch(t *testing.T) {
	runner := &recordingRunner{output: []byte(`{"config":{"agents":{"entries":{"medical":{}}}},"hash":"base"}`)}
	client := CLIClient{Runner: runner, Binary: "openclaw"}
	actual, err := client.GetAgent(context.Background(), "medical")
	if err != nil || !actual.Exists || actual.ConfigHash != "base" {
		t.Fatalf("GetAgent() = %#v, %v", actual, err)
	}
	desired := compiler.DesiredOpenClawState{AgentID: "medical", Workspace: "/tmp/workspace", Sandbox: compiler.SandboxState{Mode: "all", Backend: "docker", Scope: "agent", WorkspaceAccess: "none", DockerNetwork: "none", AllowExternalBindSources: true}, Mount: compiler.BindMount{Source: "/tmp/data", Target: "/data", ReadOnly: true}, Tools: compiler.ToolPolicy{Denied: []string{"browser", "group:web", "exec"}}, ExecHost: "sandbox"}
	if err := client.PatchAgent(context.Background(), desired, "base", []string{"write", "browser"}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 2 || runner.calls[1].args[2] != "config.patch" {
		t.Fatalf("commands = %#v", runner.calls)
	}
	params := runner.calls[1].args[4]
	var envelope struct {
		Raw string `json:"raw"`
	}
	if err := json.Unmarshal([]byte(params), &envelope); err != nil {
		t.Fatal(err)
	}
	var patch map[string]any
	if err := json.Unmarshal([]byte(envelope.Raw), &patch); err != nil {
		t.Fatal(err)
	}
	agents := patch["agents"].(map[string]any)
	entries := agents["entries"].(map[string]any)
	if len(entries) != 1 || entries["medical"] == nil {
		t.Fatalf("patch must only contain medical agent: %#v", patch)
	}
	if _, changedGlobal := patch["gateway"]; changedGlobal {
		t.Fatalf("patch unexpectedly changed global config: %#v", patch)
	}
	medical := entries["medical"].(map[string]any)
	if got := medical["workspace"]; got != desired.Workspace {
		t.Fatalf("workspace patch = %#v, want %q", got, desired.Workspace)
	}
	docker := medical["sandbox"].(map[string]any)["docker"].(map[string]any)
	if got := docker["dangerouslyAllowExternalBindSources"]; got != true {
		t.Fatalf("external bind source setting = %#v, want true", got)
	}
	if _, changedGlobal := patch["sandbox"]; changedGlobal {
		t.Fatalf("patch unexpectedly changed global sandbox config: %#v", patch)
	}
	tools := medical["tools"].(map[string]any)
	denied := stringsAt(tools, "deny")
	if !contains(denied, "write") || !contains(denied, "browser") {
		t.Fatalf("patch did not preserve unrelated or required denial: %#v", denied)
	}
}

func TestExtractAgentReadsExternalBindSourceAuthorization(t *testing.T) {
	for _, test := range []struct {
		name   string
		docker map[string]any
		want   bool
	}{
		{name: "true", docker: map[string]any{"dangerouslyAllowExternalBindSources": true}, want: true},
		{name: "false", docker: map[string]any{"dangerouslyAllowExternalBindSources": false}, want: false},
		{name: "missing", docker: map[string]any{}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := map[string]any{"agents": map[string]any{"entries": map[string]any{"medical": map[string]any{"sandbox": map[string]any{"docker": test.docker}}}}}
			actual := extractAgent(config, "hash", "medical")
			if actual.Sandbox.AllowExternalBindSources != test.want {
				t.Fatalf("AllowExternalBindSources = %t, want %t", actual.Sandbox.AllowExternalBindSources, test.want)
			}
		})
	}
}
