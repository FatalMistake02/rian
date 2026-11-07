// Package app dispatches the small CLI and creates the terminal application.
package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"rian/internal/config"
	"rian/internal/desktop"
	"rian/internal/detect"
	"rian/internal/git"
	"rian/internal/process"
	"rian/internal/project"
	"rian/internal/tools"
	"rian/internal/ui"
)

func Run(args []string, in *os.File, out io.Writer, version string) error {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "version") {
		fmt.Fprintln(out, "rian "+version)
		return nil
	}
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		fmt.Fprint(out, usage)
		return nil
	}
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "--install-app" {
		return desktop.OfferInstallation(filepath.Dir(path))
	}
	store := config.New(path)
	if len(args) == 0 {
		if !term.IsTerminal(in.Fd()) {
			return fmt.Errorf("the interface needs an interactive terminal; use 'rian list' or 'rian --help'")
		}
		if err := desktop.Offer(filepath.Dir(path), in, out); err != nil {
			fmt.Fprintln(out, "rian:", err)
		}
		logger, closeLog := openLog(filepath.Dir(path))
		defer closeLog()
		manager := process.New()
		defer manager.Close()
		_, err := tea.NewProgram(ui.New(store, manager, logger, version), tea.WithInput(in), tea.WithOutput(out)).Run()
		return err
	}
	if args[0] == "config" {
		fmt.Fprintln(out, path)
		return nil
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		return addProject(store, args[1:], in, out)
	case "list":
		if len(args) == 2 && args[1] == "--json" {
			return json.NewEncoder(out).Encode(cfg.Projects)
		}
		if len(args) != 1 {
			return fmt.Errorf("usage: rian list [--json]")
		}
		if len(cfg.Projects) == 0 {
			fmt.Fprintln(out, "No projects yet. Add one with: rian add .")
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 4, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tID\tDIRECTORY")
		for _, p := range cfg.Projects {
			name := safe(p.Name)
			if p.Favorite {
				name = "★ " + name
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", name, p.ID, safe(p.Path))
		}
		return w.Flush()
	case "remove":
		args, yes := confirmationFlag(args[1:])
		if len(args) != 1 {
			return fmt.Errorf("usage: rian remove <project> [--yes]")
		}
		p, err := project.Lookup(cfg.Projects, args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Remove %s from Rian? Its directory and files stay in place.\n", safe(p.Name))
		if err := confirm(yes, in, out); err != nil {
			return err
		}
		if _, err := store.Update(func(cfg *config.Config) error { return config.Remove(cfg, p.ID) }); err != nil {
			return err
		}
		fmt.Fprintln(out, "Project entry removed.")
		return nil
	case "open", "vscode", "terminal", "github", "opencode", "kilocode", "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: rian %s <project>", args[0])
		}
		if args[0] != "run" && len(args) != 2 || args[0] == "run" && len(args) != 3 {
			return fmt.Errorf("unexpected arguments; see rian --help")
		}
		p, err := project.Lookup(cfg.Projects, args[1])
		if err != nil {
			return err
		}
		return launch(store, p, args, in, out)
	default:
		return fmt.Errorf("unknown command %q; see rian --help", args[0])
	}
}

func addProject(store *config.Store, args []string, in *os.File, out io.Writer) error {
	args, yes := confirmationFlag(args)
	if len(args) != 1 {
		return fmt.Errorf("usage: rian add <directory> [--yes]")
	}
	result, err := detect.Inspect(args[0])
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	for _, p := range cfg.Projects {
		if project.SamePath(p.Path, result.Project.Path) {
			return fmt.Errorf("directory already saved as %s", p.Name)
		}
	}
	p := result.Project
	fmt.Fprintf(out, "\n%s\n  %s\n", safe(p.Name), safe(p.Path))
	if len(p.Stack) > 0 {
		fmt.Fprintln(out, "  Stack: "+strings.Join(p.Stack, " · "))
	}
	if result.Git.Repository {
		fmt.Fprintln(out, "  Branch: "+safe(result.Git.Branch))
	}
	if result.Git.GitHub != "" {
		fmt.Fprintln(out, "  GitHub: "+safe(result.Git.GitHub))
	}
	for _, name := range detect.CommandNames(p.AutoCommands) {
		fmt.Fprintf(out, "  %-12s %s\n", safe(name), safe(p.AutoCommands[name]))
	}
	for _, tool := range []string{"code", "opencode", "kilocode", "gh"} {
		status := "not in PATH"
		if result.Available[tool] {
			status = "available"
		}
		fmt.Fprintf(out, "  %-12s %s\n", tool, status)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(out, "  "+safe(warning))
	}
	fmt.Fprintln(out, "\nSave this project?")
	if err := confirm(yes, in, out); err != nil {
		return err
	}
	if _, err := store.Update(func(cfg *config.Config) error { return config.Add(cfg, p) }); err != nil {
		return err
	}
	fmt.Fprintf(out, "Saved %s [%s].\n", safe(p.Name), p.ID)
	return nil
}

func confirmationFlag(args []string) ([]string, bool) {
	result, yes := []string{}, false
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" {
			yes = true
		} else {
			result = append(result, arg)
		}
	}
	return result, yes
}

func confirm(yes bool, in *os.File, out io.Writer) error {
	if yes {
		return nil
	}
	if !term.IsTerminal(in.Fd()) {
		return fmt.Errorf("confirmation requires a terminal; pass --yes after reviewing the detected project")
	}
	fmt.Fprint(out, "[y/N] ")
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
		return nil
	}
	return fmt.Errorf("cancelled")
}

func launch(store *config.Store, p project.Project, args []string, in *os.File, out io.Writer) error {
	name := args[0]
	if name == "open" {
		name = "vscode"
	}
	if !p.ToolEnabled(name) {
		return fmt.Errorf("%s is disabled for this project", name)
	}
	launcher := tools.System{}
	var err error
	var cmd *exec.Cmd
	switch name {
	case "vscode":
		err = launcher.OpenEditor(p)
	case "github":
		var value string
		value, err = tools.GitHubURL(p, git.Inspect(p.Path))
		if err == nil {
			err = launcher.OpenURL(value)
		}
	case "run":
		result, detectErr := detect.Inspect(p.Path)
		if detectErr != nil {
			return detectErr
		}
		p.AutoCommands = result.Project.AutoCommands
		commands := p.AllCommands()
		command, exists := commands[args[2]]
		if !exists {
			return fmt.Errorf("command %q not found; available: %s", args[2], strings.Join(detect.CommandNames(commands), ", "))
		}
		fmt.Fprintln(out, "$ "+safe(command))
		cmd = tools.Shell(p.Path, command)
	default:
		cmd, err = launcher.Interactive(name, p)
	}
	if err != nil {
		return err
	}
	if _, err := store.Update(func(cfg *config.Config) error { return config.Touch(cfg, p.ID) }); err != nil {
		return err
	}
	if cmd != nil {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, out
		return cmd.Run()
	}
	return nil
}

func openLog(dir string) (*log.Logger, func()) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return log.New(io.Discard, "", 0), func() {}
	}
	path := filepath.Join(dir, "rian.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1024*1024 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return log.New(io.Discard, "", 0), func() {}
	}
	return log.New(f, "rian ", log.LstdFlags), func() { _ = f.Close() }
}

func safe(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, value)
}

const usage = `Rian · your local project desk

Usage:
  rian                         Open the full-screen interface
  rian add <directory> [--yes]  Inspect and save a project
  rian list [--json]            List saved projects
  rian open <project>           Open in VS Code
  rian vscode <project>         Open in VS Code
  rian terminal <project>       Open an interactive shell here
  rian opencode <project>       Launch OpenCode interactively
  rian kilocode <project>       Launch KiloCode (kilo or kilocode)
  rian github <project>         Open the GitHub repository
  rian run <project> <command>  Run a detected or custom command
  rian remove <project> [--yes] Remove only the Rian entry
  rian config                  Print the configuration path
  rian --version               Show version

Projects accept a name, ID, or unambiguous fuzzy match. Quote names with spaces.
In the interface: a add · / search · Ctrl+K actions · ? help · q quit
`
