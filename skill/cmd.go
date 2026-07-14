package skill

import (
	"flag"
	"fmt"
	"os"
)

// Execute runs the skill subcommand group.
func Execute(args []string) {
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "install":
		RunInstall(cmdArgs)
	case "update":
		RunUpdate(cmdArgs)
	case "remove":
		RunRemove(cmdArgs)
	case "list":
		RunList(cmdArgs)
	case "inspect", "info":
		RunInspect(cmdArgs)
	case "doctor":
		RunDoctor(cmdArgs)
	case "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown skill command: %s\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: skill <command> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  install <source> [name]   Install a skill\n")
	fmt.Fprintf(os.Stderr, "  update [name]             Update a skill (or --all)\n")
	fmt.Fprintf(os.Stderr, "  remove <name>             Remove a skill\n")
	fmt.Fprintf(os.Stderr, "  list                      List installed skills\n")
	fmt.Fprintf(os.Stderr, "  inspect <name>            Inspect an installed skill\n")
	fmt.Fprintf(os.Stderr, "  doctor                    Check for issues with installed skills\n")
	fmt.Fprintf(os.Stderr, "\n")
}

func parseScope(scopeStr string) (Scope, error) {
	switch scopeStr {
	case "user":
		return ScopeUser, nil
	case "project":
		return ScopeProject, nil
	default:
		return "", fmt.Errorf("invalid scope: %s (must be 'user' or 'project')", scopeStr)
	}
}

func RunInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	scopeStr := fs.String("scope", "project", "Installation scope: 'user' or 'project'")
	agentTarget := fs.String("agent", "", "Target agent (e.g., codex, claude, copilot, cursor)")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: skill install <source> [name] [flags]\n")
		os.Exit(1)
	}

	source := fs.Arg(0)
	name := fs.Arg(1)

	scope, err := parseScope(*scopeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	err = InstallSkill(source, name, scope, *agentTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to install skill: %v\n", err)
		os.Exit(1)
	}
}

func RunUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	all := fs.Bool("all", false, "Update all skills")
	force := fs.Bool("force", false, "Force update even if locally modified")
	scopeStr := fs.String("scope", "project", "Installation scope: 'user' or 'project'")
	agentTarget := fs.String("agent", "", "Target agent")
	_ = fs.Parse(args)

	scope, err := parseScope(*scopeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *all {
		err = UpdateAllSkills(scope, *agentTarget, *force)
	} else {
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: skill update <name> [flags] or --all\n")
			os.Exit(1)
		}
		name := fs.Arg(0)
		err = UpdateSkill(name, scope, *agentTarget, *force)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update skill(s): %v\n", err)
		os.Exit(1)
	}
}

func RunRemove(args []string) {
	fs := flag.NewFlagSet("remove", flag.ExitOnError)
	scopeStr := fs.String("scope", "project", "Installation scope: 'user' or 'project'")
	agentTarget := fs.String("agent", "", "Target agent")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: skill remove <name> [flags]\n")
		os.Exit(1)
	}

	name := fs.Arg(0)
	scope, err := parseScope(*scopeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	err = RemoveSkill(name, scope, *agentTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove skill: %v\n", err)
		os.Exit(1)
	}
}

func RunList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	scopeStr := fs.String("scope", "project", "Installation scope: 'user' or 'project'")
	agentTarget := fs.String("agent", "", "Target agent")
	format := fs.String("format", "text", "Output format (text, json)")
	_ = fs.Parse(args)

	scope, err := parseScope(*scopeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	err = ListSkills(scope, *agentTarget, *format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list skills: %v\n", err)
		os.Exit(1)
	}
}

func RunInspect(args []string) {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	scopeStr := fs.String("scope", "project", "Installation scope: 'user' or 'project'")
	agentTarget := fs.String("agent", "", "Target agent")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: skill inspect <name> [flags]\n")
		os.Exit(1)
	}

	name := fs.Arg(0)
	scope, err := parseScope(*scopeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	err = InspectSkill(name, scope, *agentTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to inspect skill: %v\n", err)
		os.Exit(1)
	}
}

func RunDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	scopeStr := fs.String("scope", "project", "Installation scope: 'user' or 'project'")
	agentTarget := fs.String("agent", "", "Target agent")
	_ = fs.Parse(args)

	scope, err := parseScope(*scopeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	err = DoctorSkills(scope, *agentTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Doctor found issues: %v\n", err)
		os.Exit(1)
	}
}
