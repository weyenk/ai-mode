package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type command struct {
	help string
	run  func(args []string) int
}

func commands() map[string]command {
	return map[string]command{
		"list":    {"List discovered profiles", cmdList},
		"which":   {"Show active profile", cmdWhich},
		"status":  {"Alias for which", cmdWhich},
		"use":     {"Activate a profile (start llama-server)", cmdUse},
		"warm":    {"Prefetch/load models on the active router", cmdWarm},
		"stop":    {"Stop the managed server", cmdStop},
		"restart": {"Restart active (or named) profile", cmdRestart},
		"doctor":  {"Check install, profiles, and server health", cmdDoctor},
		"env":     {`Print shell exports for eval "$(ai-mode env)"`, cmdEnv},
		"logs":    {"Tail profile logs", cmdLogs},
		"url":     {"Print active OpenAI base URL", cmdURL},
		"models":  {"List models from the active server", cmdModels},
		"init":    {"Scaffold a new profile (.mode and optional .ini)", cmdInit},
		"agents":  {"List agent markdown guides", cmdAgents},
		"prompt":  {"Print system prompt for a role (agents/<role>.md)", cmdPrompt},
		"ask":     {"Ask a role one question (chat/completions)", cmdAsk},
		"events":  {"Show the structured event log (calls, lifecycle)", cmdEvents},
		"trace":   {"Show a call tree for a trace id (default: latest)", cmdTrace},
		"stats":   {"Latency, token and error stats per role", cmdStats},
		"ps":      {"Loaded/unloaded models with last-call info", cmdPS},
	}
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s [--presets DIR] <command> [args]\n\nSwitch and manage llama-server AI profile presets.\n\ncommands:\n", appName)
	cmds := commands()
	names := make([]string, 0, len(cmds))
	for n := range cmds {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "  %-9s %s\n", n, cmds[n].help)
	}
}

// Main dispatches argv (without the program name) and returns the exit code.
func Main(argv []string) int {
	// Global --presets may precede the subcommand.
	for len(argv) > 0 && strings.HasPrefix(argv[0], "-") {
		switch {
		case argv[0] == "-h" || argv[0] == "--help" || argv[0] == "-help":
			usage(os.Stdout)
			return 0
		case argv[0] == "--presets" && len(argv) > 1:
			os.Setenv("AI_MODE_PRESETS", argv[1])
			argv = argv[2:]
		case strings.HasPrefix(argv[0], "--presets="):
			os.Setenv("AI_MODE_PRESETS", strings.TrimPrefix(argv[0], "--presets="))
			argv = argv[1:]
		default:
			fmt.Fprintf(os.Stderr, "unknown option: %s\n", argv[0])
			usage(os.Stderr)
			return 2
		}
	}
	if len(argv) == 0 {
		usage(os.Stderr)
		return 2
	}
	cmd, ok := commands()[argv[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", argv[0])
		usage(os.Stderr)
		return 2
	}
	return cmd.run(argv[1:])
}

// newFlags makes a FlagSet that reports errors to stderr without exiting.
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(appName+" "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// parseArgs parses flags that may appear before, between, or after positionals
// (argparse-style); Go's flag package alone stops at the first positional.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// parse runs parseArgs and maps flag errors to an exit code (ok=false).
func parse(fs *flag.FlagSet, args []string) (pos []string, code int, ok bool) {
	pos, err := parseArgs(fs, args)
	if err == flag.ErrHelp {
		return nil, 0, false
	}
	if err != nil {
		return nil, 2, false
	}
	return pos, 0, true
}

func secs(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	return 1
}

// activeProfile resolves --profile / active state to a Profile, printing errors.
func activeProfile(name string, hint string) (Profile, int) {
	if name == "" {
		name = loadState().Profile
	}
	if name == "" {
		return Profile{}, fail("%s", strings.TrimSpace("No active profile. "+hint))
	}
	p, ok := getProfile(name)
	if !ok {
		return Profile{}, fail("Unknown profile: %s", name)
	}
	return p, 0
}
