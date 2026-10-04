package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pvclaw/pvclaw/internal/compiler"
	"github.com/pvclaw/pvclaw/internal/openclaw"
	"github.com/pvclaw/pvclaw/internal/plan"
	"github.com/pvclaw/pvclaw/internal/profile"
	"github.com/pvclaw/pvclaw/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "plan":
		return runPlan(args[1:], stdout, stderr)
	case "create":
		return runCreate(args[1:], stdout, stderr)
	case "diff":
		return runDiff(args[1:], stdout, stderr)
	case "apply":
		return runApply(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runCreate(args []string, stdout, stderr io.Writer) int {
	home, agentID, template, _, code := parseRuntimeArgs("create", args, stderr)
	if code != 0 {
		return code
	}
	if template == "" {
		fmt.Fprintln(stderr, "error: --template medical|work is required")
		return 2
	}
	paths, err := (store.Store{Home: home}).Create(agentID, template)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Created PVClaw agent %s\n  Profile: %s\n  Data directory: %s\n", agentID, paths.ProfilePath, paths.DataDirectory)
	return 0
}

func runDiff(args []string, stdout, stderr io.Writer) int {
	home, agentID, template, yes, code := parseRuntimeArgs("diff", args, stderr)
	if code != 0 {
		return code
	}
	if template != "" || yes {
		fmt.Fprintln(stderr, "error: diff does not accept --template or --yes")
		return 2
	}
	reconciler := openclaw.Reconciler{Store: store.Store{Home: home}}
	diff, err := reconciler.Diff(context.Background(), agentID)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprint(stdout, openclaw.FormatDiff(diff))
	return 0
}

func runApply(args []string, stdout, stderr io.Writer) int {
	home, agentID, template, yes, code := parseRuntimeArgs("apply", args, stderr)
	if code != 0 {
		return code
	}
	if template != "" {
		fmt.Fprintln(stderr, "error: apply does not accept --template")
		return 2
	}
	if !yes {
		fmt.Fprintf(stderr, "Apply PVClaw profile for %q to OpenClaw? [y/N]: ", agentID)
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil && len(answer) == 0 {
			fmt.Fprintln(stderr, "\nerror: confirmation was not received")
			return 1
		}
		if strings.ToLower(strings.TrimSpace(answer)) != "y" && strings.ToLower(strings.TrimSpace(answer)) != "yes" {
			fmt.Fprintln(stderr, "Apply cancelled.")
			return 0
		}
	}
	reconciler := openclaw.Reconciler{Store: store.Store{Home: home}}
	result, err := reconciler.Apply(context.Background(), agentID)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprint(stdout, openclaw.FormatDiff(result.Diff))
	fmt.Fprintln(stdout, "OpenClaw reconciliation succeeded.")
	if result.Created {
		fmt.Fprintln(stdout, "OpenClaw agent: created")
	}
	if result.Patched {
		fmt.Fprintln(stdout, "OpenClaw configuration: patched")
	}
	if result.Recreated {
		fmt.Fprintln(stdout, "Sandbox: recreated")
	}
	return 0
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	_, profilePath, code := parseCommandArgs("validate", args, stderr)
	if code != 0 {
		return code
	}

	p, err := profile.LoadFile(profilePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if err := profile.Validate(p); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "Profile is valid.")
	return 0
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	home, profilePath, code := parseCommandArgs("plan", args, stderr)
	if code != 0 {
		return code
	}

	p, err := profile.LoadFile(profilePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	desired, err := compiler.Compile(p, home)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprint(stdout, plan.Format(desired))
	return 0
}

func parseCommandArgs(command string, args []string, stderr io.Writer) (string, string, int) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	home := flags.String("home", defaultHome(), "PVClaw home directory")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "Usage: pvclaw %s [--home <path>] <profile.yaml>\n", command)
	}
	if err := flags.Parse(args); err != nil {
		return "", "", 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return "", "", 2
	}
	return *home, flags.Arg(0), 0
}

func parseRuntimeArgs(command string, args []string, stderr io.Writer) (home, agentID, template string, yes bool, code int) {
	home = defaultHome()
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--home":
			if index+1 >= len(args) {
				return runtimeUsage(command, stderr)
			}
			index++
			home = args[index]
		case "--template":
			if index+1 >= len(args) {
				return runtimeUsage(command, stderr)
			}
			index++
			template = args[index]
		case "--yes":
			yes = true
		default:
			if strings.HasPrefix(argument, "-") || agentID != "" {
				return runtimeUsage(command, stderr)
			}
			agentID = argument
		}
	}
	if agentID == "" {
		return runtimeUsage(command, stderr)
	}
	return home, agentID, template, yes, 0
}

func runtimeUsage(command string, stderr io.Writer) (string, string, string, bool, int) {
	switch command {
	case "create":
		fmt.Fprintln(stderr, "Usage: pvclaw create <agent-id> --template medical|work [--home <path>]")
	case "diff":
		fmt.Fprintln(stderr, "Usage: pvclaw diff <agent-id> [--home <path>]")
	case "apply":
		fmt.Fprintln(stderr, "Usage: pvclaw apply <agent-id> [--yes] [--home <path>]")
	}
	return "", "", "", false, 2
}

func defaultHome() string {
	if home := os.Getenv("PVCLAW_HOME"); home != "" {
		return home
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return ".pvclaw"
	}
	return filepath.Join(userHome, ".pvclaw")
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  pvclaw validate <profile.yaml>")
	fmt.Fprintln(w, "  pvclaw plan [--home <path>] <profile.yaml>")
	fmt.Fprintln(w, "  pvclaw create <agent-id> --template medical|work [--home <path>]")
	fmt.Fprintln(w, "  pvclaw diff <agent-id> [--home <path>]")
	fmt.Fprintln(w, "  pvclaw apply <agent-id> [--yes] [--home <path>]")
}
