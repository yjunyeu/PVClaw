package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/pvclaw/pvclaw/internal/compiler"
)

// CommandRunner isolates process execution from reconciliation and unit tests.
type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

// CLIClient calls the documented OpenClaw CLI and Gateway config RPC only.
type CLIClient struct {
	Runner CommandRunner
	Binary string
}

func (c CLIClient) GetAgent(ctx context.Context, agentID string) (ActualAgentState, error) {
	output, err := c.runner().Run(ctx, c.binary(), "gateway", "call", "config.get", "--params", "{}", "--json")
	if err != nil {
		return ActualAgentState{}, err
	}
	var response configGetResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return ActualAgentState{}, fmt.Errorf("decode OpenClaw config.get response: %w", err)
	}
	return extractAgent(response.Config, response.Hash, agentID), nil
}

func (c CLIClient) AddAgent(ctx context.Context, agentID, workspace string) error {
	_, err := c.runner().Run(ctx, c.binary(), "agents", "add", agentID, "--workspace", workspace, "--non-interactive")
	return err
}

func (c CLIClient) PatchAgent(ctx context.Context, desired compiler.DesiredOpenClawState, baseHash string, existingDenied []string) error {
	if baseHash == "" {
		return fmt.Errorf("OpenClaw config.get returned no base hash")
	}
	patch := agentPatch(desired, existingDenied)
	raw, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("encode OpenClaw agent patch: %w", err)
	}
	params, err := json.Marshal(map[string]any{
		"raw":          string(raw),
		"baseHash":     baseHash,
		"replacePaths": agentReplacePaths(desired.AgentID),
		"note":         "PVClaw managed agent reconciliation",
	})
	if err != nil {
		return fmt.Errorf("encode config.patch parameters: %w", err)
	}
	_, err = c.runner().Run(ctx, c.binary(), "gateway", "call", "config.patch", "--params", string(params), "--json")
	return err
}

func (c CLIClient) RecreateSandbox(ctx context.Context, agentID string) error {
	_, err := c.runner().Run(ctx, c.binary(), "sandbox", "recreate", "--agent", agentID, "--force")
	return err
}

func (c CLIClient) ExplainSandbox(ctx context.Context, agentID string) (string, error) {
	output, err := c.runner().Run(ctx, c.binary(), "sandbox", "explain", "--agent", agentID, "--json")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(output)) == "" {
		return "", fmt.Errorf("OpenClaw sandbox explain returned no output")
	}
	return string(output), nil
}

func (c CLIClient) runner() CommandRunner {
	if c.Runner != nil {
		return c.Runner
	}
	return ExecRunner{}
}

func (c CLIClient) binary() string {
	if c.Binary != "" {
		return c.Binary
	}
	return "openclaw"
}

type configGetResponse struct {
	Config map[string]any `json:"config"`
	Hash   string         `json:"hash"`
}

func agentPatch(desired compiler.DesiredOpenClawState, existingDenied []string) map[string]any {
	mode := "rw"
	if desired.Mount.ReadOnly {
		mode = "ro"
	}
	return map[string]any{
		"agents": map[string]any{
			"entries": map[string]any{
				desired.AgentID: map[string]any{
					"sandbox": map[string]any{
						"mode":            desired.Sandbox.Mode,
						"backend":         desired.Sandbox.Backend,
						"scope":           desired.Sandbox.Scope,
						"workspaceAccess": desired.Sandbox.WorkspaceAccess,
						"docker": map[string]any{
							"network": desired.Sandbox.DockerNetwork,
							"binds":   []string{fmt.Sprintf("%s:%s:%s", desired.Mount.Source, desired.Mount.Target, mode)},
						},
						"browser": map[string]any{"enabled": desired.Sandbox.BrowserEnabled},
					},
					"tools": map[string]any{
						"deny":     mergeDenied(existingDenied, desired.Tools.Denied),
						"elevated": map[string]any{"enabled": false},
						"exec":     map[string]any{"host": desired.ExecHost},
					},
				},
			},
		},
	}
}

// mergeDenied only replaces PVClaw's three capability decisions. Other
// existing denials stay in place so reconciliation never removes an unrelated
// restriction from a managed agent.
func mergeDenied(existing, desired []string) []string {
	pvclawTools := map[string]bool{"browser": true, "group:web": true, "exec": true}
	values := make(map[string]bool, len(existing)+len(desired))
	for _, value := range existing {
		if !pvclawTools[value] {
			values[value] = true
		}
	}
	for _, value := range desired {
		values[value] = true
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func agentReplacePaths(agentID string) []string {
	return []string{
		fmt.Sprintf("agents.entries.%s.sandbox.docker.binds", agentID),
		fmt.Sprintf("agents.entries.%s.tools.deny", agentID),
	}
}

func extractAgent(config map[string]any, hash, agentID string) ActualAgentState {
	entries := objectAt(config, "agents", "entries")
	entry, exists := entries[agentID].(map[string]any)
	if !exists {
		return ActualAgentState{AgentID: agentID, ConfigHash: hash}
	}
	sandbox := objectAt(entry, "sandbox")
	docker := objectAt(sandbox, "docker")
	browser := objectAt(sandbox, "browser")
	tools := objectAt(entry, "tools")
	elevated := objectAt(tools, "elevated")
	execConfig := objectAt(tools, "exec")
	denied := stringsAt(tools, "deny")
	return ActualAgentState{
		Exists:     true,
		AgentID:    agentID,
		ConfigHash: hash,
		Sandbox: ActualSandboxState{
			Mode:            stringAt(sandbox, "mode"),
			Backend:         stringAt(sandbox, "backend"),
			Scope:           stringAt(sandbox, "scope"),
			WorkspaceAccess: stringAt(sandbox, "workspaceAccess"),
			DockerNetwork:   stringAt(docker, "network"),
			BrowserEnabled:  boolAt(browser, "enabled"),
		},
		Mount: findDataMount(stringsAt(docker, "binds")),
		Tools: ActualToolState{
			Denied:        denied,
			BrowserDenied: contains(denied, "browser"),
			WebDenied:     contains(denied, "group:web"),
			ShellDenied:   contains(denied, "exec"),
			// Omission is treated conservatively: PVClaw will explicitly set this
			// false, so an unknown/default elevated setting must never look safe.
			ElevatedOn: boolWithDefault(elevated, "enabled", true),
			ExecHost:   stringAt(execConfig, "host"),
		},
	}
}

func objectAt(value map[string]any, keys ...string) map[string]any {
	current := value
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			return map[string]any{}
		}
		current = next
	}
	return current
}

func stringAt(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return result
}

func boolAt(value map[string]any, key string) bool {
	result, _ := value[key].(bool)
	return result
}

func boolWithDefault(value map[string]any, key string, fallback bool) bool {
	result, ok := value[key].(bool)
	if !ok {
		return fallback
	}
	return result
}

func stringsAt(value map[string]any, key string) []string {
	values, _ := value[key].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if stringValue, ok := value.(string); ok {
			result = append(result, stringValue)
		}
	}
	return result
}

func findDataMount(binds []string) ActualBindMount {
	for _, bind := range binds {
		parts := strings.Split(bind, ":")
		if len(parts) < 2 || parts[1] != "/data" {
			continue
		}
		return ActualBindMount{
			Found:    true,
			Source:   parts[0],
			Target:   parts[1],
			ReadOnly: len(parts) >= 3 && parts[2] == "ro",
		}
	}
	return ActualBindMount{}
}
