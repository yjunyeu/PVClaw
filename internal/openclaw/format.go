package openclaw

import (
	"fmt"
	"strings"
)

// FormatDiff returns stable, human-readable reconciliation output.
func FormatDiff(diff Diff) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Agent: %s\n", diff.AgentID)
	if diff.Empty() {
		builder.WriteString("OpenClaw: synchronized\n")
		return builder.String()
	}
	builder.WriteString("OpenClaw changes:\n")
	for _, change := range diff.Changes {
		fmt.Fprintf(
			&builder,
			"  [%s] %s: %s -> %s\n",
			change.Kind,
			change.Field,
			change.Actual,
			change.Desired,
		)
	}
	return builder.String()
}
