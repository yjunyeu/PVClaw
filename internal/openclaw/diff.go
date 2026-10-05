package openclaw

import (
	"fmt"

	"github.com/pvclaw/pvclaw/internal/compiler"
)

// Compare produces a stable field-by-field comparison of desired and actual state.
func Compare(desired compiler.DesiredOpenClawState, actual ActualAgentState) Diff {
	diff := Diff{AgentID: desired.AgentID}
	if !actual.Exists {
		return Diff{AgentID: desired.AgentID, Changes: []Change{{
			Field: "agent", Desired: "present", Actual: "missing", Kind: ChangeCreate,
		}}}
	}

	appendChange(&diff, "workspace", desired.Workspace, actual.Workspace, ChangeConfiguration)
	appendChange(&diff, "sandbox.mode", desired.Sandbox.Mode, actual.Sandbox.Mode, ChangeConfiguration)
	appendChange(&diff, "sandbox.backend", desired.Sandbox.Backend, actual.Sandbox.Backend, ChangeConfiguration)
	appendChange(&diff, "sandbox.scope", desired.Sandbox.Scope, actual.Sandbox.Scope, ChangeConfiguration)
	appendChange(&diff, "sandbox.workspace_access", desired.Sandbox.WorkspaceAccess, actual.Sandbox.WorkspaceAccess, ChangeConfiguration)
	appendChange(&diff, "sandbox.docker.network", desired.Sandbox.DockerNetwork, actual.Sandbox.DockerNetwork, networkKind(desired.Sandbox.DockerNetwork, actual.Sandbox.DockerNetwork))
	appendChange(&diff, "sandbox.docker.dangerously_allow_external_bind_sources", boolString(desired.Sandbox.AllowExternalBindSources), boolString(actual.Sandbox.AllowExternalBindSources), boolKind(desired.Sandbox.AllowExternalBindSources, actual.Sandbox.AllowExternalBindSources))
	appendChange(&diff, "mount./data", formatMount(desired.Mount.Source, desired.Mount.ReadOnly), formatActualMount(actual.Mount), mountKind(desired.Mount, actual.Mount))
	appendChange(&diff, "sandbox.browser.enabled", boolString(desired.Sandbox.BrowserEnabled), boolString(actual.Sandbox.BrowserEnabled), boolKind(desired.Sandbox.BrowserEnabled, actual.Sandbox.BrowserEnabled))
	appendChange(&diff, "tools.browser", toolState(desired.Tools.Denied, "browser"), boolToolState(actual.Tools.BrowserDenied), toolKind(desired.Tools.Denied, "browser", actual.Tools.BrowserDenied))
	appendChange(&diff, "tools.web_search", toolState(desired.Tools.Denied, "group:web"), boolToolState(actual.Tools.WebDenied), toolKind(desired.Tools.Denied, "group:web", actual.Tools.WebDenied))
	appendChange(&diff, "tools.shell", toolState(desired.Tools.Denied, "exec"), boolToolState(actual.Tools.ShellDenied), toolKind(desired.Tools.Denied, "exec", actual.Tools.ShellDenied))
	appendChange(&diff, "tools.elevated", "disabled", boolString(actual.Tools.ElevatedOn), elevatedKind(actual.Tools.ElevatedOn))
	appendChange(&diff, "tools.exec.host", desired.ExecHost, actual.Tools.ExecHost, execHostKind(desired.ExecHost, actual.Tools.ExecHost))
	return diff
}

func appendChange(diff *Diff, field, desired, actual string, kind ChangeKind) {
	if desired != actual {
		diff.Changes = append(diff.Changes, Change{Field: field, Desired: desired, Actual: actual, Kind: kind})
	}
}

func formatMount(source string, readOnly bool) string {
	mode := "rw"
	if readOnly {
		mode = "ro"
	}
	return fmt.Sprintf("%s -> /data [%s]", source, mode)
}

func formatActualMount(mount ActualBindMount) string {
	if !mount.Found {
		return "missing"
	}
	return formatMount(mount.Source, mount.ReadOnly)
}

func mountKind(desired compiler.BindMount, actual ActualBindMount) ChangeKind {
	if !actual.Found {
		return ChangeConfiguration
	}
	if desired.ReadOnly && !actual.ReadOnly {
		return ChangeRestrictive
	}
	if !desired.ReadOnly && actual.ReadOnly {
		return ChangeBroadening
	}
	return ChangeConfiguration
}

func networkKind(desired, actual string) ChangeKind {
	if desired == "none" && actual != "none" {
		return ChangeRestrictive
	}
	if desired != "none" && actual == "none" {
		return ChangeBroadening
	}
	return ChangeConfiguration
}

func boolKind(desired, actual bool) ChangeKind {
	if !desired && actual {
		return ChangeRestrictive
	}
	if desired && !actual {
		return ChangeBroadening
	}
	return ChangeConfiguration
}

func toolKind(denied []string, tool string, actualDenied bool) ChangeKind {
	desiredDenied := contains(denied, tool)
	if desiredDenied && !actualDenied {
		return ChangeRestrictive
	}
	if !desiredDenied && actualDenied {
		return ChangeBroadening
	}
	return ChangeConfiguration
}

func elevatedKind(actualEnabled bool) ChangeKind {
	if actualEnabled {
		return ChangeRestrictive
	}
	return ChangeConfiguration
}

func execHostKind(desired, actual string) ChangeKind {
	if desired == "sandbox" && actual != "sandbox" {
		return ChangeRestrictive
	}
	return ChangeConfiguration
}

func toolState(denied []string, tool string) string {
	return boolToolState(contains(denied, tool))
}

func boolToolState(denied bool) string {
	if denied {
		return "denied"
	}
	return "allowed"
}

func boolString(value bool) string {
	if value {
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
