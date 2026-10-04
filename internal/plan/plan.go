// Package plan renders deterministic, human-readable desired-state plans.
package plan

import (
	"fmt"
	"strings"

	"github.com/pvclaw/pvclaw/internal/compiler"
)

func Format(desired compiler.DesiredState) string {
	mountMode := "rw"
	access := "read-write"
	if desired.OpenClaw.Mount.ReadOnly {
		mountMode = "ro"
		access = "read-only"
	}

	var output strings.Builder
	fmt.Fprintf(&output, "Agent: %s\n", desired.OpenClaw.AgentID)
	fmt.Fprintf(&output, "Purpose: %s\n\n", desired.OpenClaw.Purpose)
	fmt.Fprintln(&output, "PVClaw")
	fmt.Fprintf(&output, "  Data directory: %s\n", desired.PVClaw.DataDirectory)
	fmt.Fprintf(&output, "  Data access: %s\n\n", access)
	fmt.Fprintln(&output, "OpenClaw")
	fmt.Fprintf(&output, "  Sandbox: %s\n", desired.OpenClaw.Sandbox.Backend)
	fmt.Fprintf(&output, "  Scope: %s\n", desired.OpenClaw.Sandbox.Scope)
	fmt.Fprintf(&output, "  Network: %s\n", enabledLabel(desired.OpenClaw.Sandbox.DockerNetwork != "none"))
	fmt.Fprintf(&output, "  Mount: %s -> %s [%s]\n", desired.OpenClaw.Mount.Source, desired.OpenClaw.Mount.Target, mountMode)
	fmt.Fprintf(&output, "  Browser: %s\n", enabledLabel(desired.OpenClaw.Sandbox.BrowserEnabled))
	fmt.Fprintf(&output, "  Web search: %s\n", enabledLabel(!contains(desired.OpenClaw.Tools.Denied, "group:web")))
	fmt.Fprintf(&output, "  Shell: %s\n", enabledLabel(!contains(desired.OpenClaw.Tools.Denied, "exec")))
	fmt.Fprintf(&output, "  Elevated: %s\n\n", enabledLabel(desired.OpenClaw.Elevated.Enabled))
	fmt.Fprintln(&output, "DefenseClaw")
	fmt.Fprintf(&output, "  Base policy: %s\n", desired.DefenseClaw.BasePolicy)
	fmt.Fprintf(&output, "  Overlay/profile: %s\n\n", desired.DefenseClaw.PolicyProfile)
	fmt.Fprintln(&output, "No runtime changes will be made.")
	return output.String()
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
