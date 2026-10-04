package plan_test

import (
	"testing"

	"github.com/pvclaw/pvclaw/internal/compiler"
	"github.com/pvclaw/pvclaw/internal/plan"
	"github.com/pvclaw/pvclaw/internal/profile"
)

func TestFormatIsDeterministic(t *testing.T) {
	t.Parallel()
	p := profile.Profile{
		Version: 1,
		Agent:   profile.Agent{ID: "medical", Name: "Medical Agent", Purpose: "medical"},
		Data:    profile.Data{Access: "read-only"},
		Policy:  profile.Policy{Profile: "medical"},
	}
	desired, err := compiler.Compile(p, "/tmp/test")
	if err != nil {
		t.Fatal(err)
	}
	const want = "Agent: medical\n" +
		"Purpose: medical\n\n" +
		"PVClaw\n" +
		"  Data directory: /tmp/test/agents/medical/data\n" +
		"  Data access: read-only\n\n" +
		"OpenClaw\n" +
		"  Sandbox: docker\n" +
		"  Scope: agent\n" +
		"  Network: disabled\n" +
		"  Mount: /tmp/test/agents/medical/data -> /data [ro]\n" +
		"  Browser: disabled\n" +
		"  Web search: disabled\n" +
		"  Shell: disabled\n" +
		"  Elevated: disabled\n\n" +
		"DefenseClaw\n" +
		"  Base policy: default\n" +
		"  Overlay/profile: medical\n\n" +
		"No runtime changes will be made.\n"
	if got := plan.Format(desired); got != want {
		t.Fatalf("plan output mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}
