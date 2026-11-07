package ui

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"rian/internal/git"
	"rian/internal/project"
	"rian/internal/tools"
)

type remoteReadyMsg struct {
	id     string
	status git.Status
	github bool
}

func (m *Model) startRemote(p project.Project) tea.Cmd {
	m.mode = remoteMenu
	m.menu = nil
	m.query = input("Search options")
	m.busy = true
	m.remoteProject = p
	m.resize()
	return func() tea.Msg {
		status := git.Inspect(p.Path)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "gh", "auth", "status", "--hostname", "github.com")
		cmd.Env = append(cmd.Environ(), "GH_PROMPT_DISABLED=1")
		err := cmd.Run()
		return remoteReadyMsg{p.ID, status, err == nil}
	}
}
func (m *Model) finishRemote(msg remoteReadyMsg) tea.Cmd {
	m.busy = false
	if m.mode != remoteMenu || msg.id != m.remoteProject.ID {
		return nil
	}
	m.setGitStatus(msg.id, msg.status)
	if !msg.status.Repository || msg.status.Error != "" {
		m.closeModal()
		return m.notify("Remote unavailable", fmt.Errorf("unable to inspect repository"))
	}
	m.remoteNames = nil
	for _, remote := range msg.status.Remotes {
		m.remoteNames = append(m.remoteNames, remote.Name)
	}
	m.menu = []Action{{ID: "remoteChoice", Label: "Use existing remote URL", Value: "existing"}}
	if msg.github {
		m.menu = append(m.menu, Action{ID: "remoteChoice", Label: "Create private GitHub repository", Value: "private"}, Action{ID: "remoteChoice", Label: "Create public GitHub repository", Value: "public"})
	}
	m.menuIndex = 0
	m.resize()
	return m.query.Focus()
}
func (m *Model) startRemoteForm(kind string) {
	m.mode = remoteForm
	m.remoteKind = kind
	m.remoteField = 0
	m.remoteInputs[0] = input("Remote name")
	m.remoteInputs[0].SetValue("origin")
	m.remoteInputs[1] = input("Remote URL")
	for _, name := range m.remoteNames {
		if name == "origin" {
			m.remoteInputs[0].SetValue("upstream")
		}
	}
	if kind != "existing" {
		m.remoteInputs[1] = input("Repository name or owner/name")
		name := regexp.MustCompile(`[^a-zA-Z0-9_.-]+`).ReplaceAllString(m.remoteProject.Name, "-")
		m.remoteInputs[1].SetValue(strings.Trim(name, "-."))
	}
	m.resize()
}
func validateRemote(name, target, kind string, names []string) error {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`).MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("enter a remote name using letters, numbers, dots, underscores or hyphens")
	}
	for _, existing := range names {
		if existing == name {
			return fmt.Errorf("remote %s already exists; choose a different name", name)
		}
	}
	if target == "" || strings.ContainsAny(target, "\x00\r\n") || strings.HasPrefix(target, "-") {
		return fmt.Errorf("enter a valid remote URL or repository name")
	}
	if kind != "existing" {
		parts := strings.Split(target, "/")
		if len(parts) > 2 {
			return fmt.Errorf("use a repository name or owner/name")
		}
		for _, part := range parts {
			if part == "." || part == ".." || !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(part) {
				return fmt.Errorf("use a repository name or owner/name")
			}
		}
		return nil
	}
	if regexp.MustCompile(`^[^\s@]+@[^\s:]+:.+$`).MatchString(target) {
		return nil
	}
	u, err := url.Parse(target)
	if err == nil {
		if u.Scheme == "file" && u.Path != "" {
			return nil
		}
		if (u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "ssh" || u.Scheme == "git") && u.Hostname() != "" {
			return nil
		}
	}
	return fmt.Errorf("use an HTTPS, SSH, Git, or file remote URL")
}
func (m *Model) remoteFormKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "esc" {
		if m.mode == remoteConfirm {
			m.mode = remoteForm
			return m.remoteInputs[m.remoteField].Focus()
		}
		m.closeModal()
		return nil
	}
	if m.mode == remoteConfirm {
		if key == "enter" {
			p := m.remoteProject
			name, target, kind := m.remoteName, m.remoteTarget, m.remoteKind
			if kind == "existing" {
				return m.runGit(p, "Add remote", [][]string{{"remote", "add", name, target}})
			}
			return m.runAddonOutput(p, "Create "+kind+" GitHub repository and add remote", func() (*exec.Cmd, error) {
				return prepareCreateRemote(p, kind, name, target)
			})
		}
		return nil
	}
	switch key {
	case "tab", "down":
		m.remoteInputs[m.remoteField].Blur()
		m.remoteField = (m.remoteField + 1) % 2
		return m.remoteInputs[m.remoteField].Focus()
	case "shift+tab", "up":
		m.remoteInputs[m.remoteField].Blur()
		m.remoteField = (m.remoteField + 1) % 2
		return m.remoteInputs[m.remoteField].Focus()
	case "enter":
		if m.remoteField == 0 {
			m.remoteInputs[0].Blur()
			m.remoteField = 1
			return m.remoteInputs[1].Focus()
		}
		name, target := strings.TrimSpace(m.remoteInputs[0].Value()), strings.TrimSpace(m.remoteInputs[1].Value())
		if err := validateRemote(name, target, m.remoteKind, m.remoteNames); err != nil {
			return m.notify("Invalid remote", err)
		}
		m.remoteName, m.remoteTarget = name, target
		m.mode = remoteConfirm
		return nil
	}
	var cmd tea.Cmd
	m.remoteInputs[m.remoteField], cmd = m.remoteInputs[m.remoteField].Update(msg)
	return cmd
}
func (m *Model) remoteFormView() (string, string, string) {
	if m.mode == remoteConfirm {
		body := "Add remote " + oneLine(m.remoteName) + "\n" + oneLine(m.remoteTarget)
		if m.remoteKind != "existing" {
			body = "Create " + m.remoteKind + " GitHub repository\n" + oneLine(m.remoteTarget) + "\n\nAdd it as remote " + oneLine(m.remoteName) + ".\nLocal commits will not be pushed."
		}
		return "CONFIRM REMOTE", body, keyHint("Esc", "back") + "  " + keyHint("Enter", "confirm")
	}
	title := "ADD REMOTE"
	label := "Remote URL"
	if m.remoteKind != "existing" {
		title = "CREATE " + strings.ToUpper(m.remoteKind) + " REPOSITORY"
		label = "Repository"
	}
	body := muted.Render("Remote name") + "\n" + m.remoteInputs[0].View() + "\n\n" + muted.Render(label) + "\n" + m.remoteInputs[1].View()
	return title, body, keyHint("Esc", "back") + "\n" + keyHint("Tab / ↑↓", "field") + "  " + keyHint("Enter", "continue")
}

func prepareCreateRemote(p project.Project, kind, name, target string) (*exec.Cmd, error) {
	if kind != "private" && kind != "public" {
		return nil, fmt.Errorf("choose private or public visibility")
	}
	status := git.Inspect(p.Path)
	if !status.Repository || status.Error != "" {
		return nil, fmt.Errorf("unable to inspect repository")
	}
	names := []string{}
	for _, remote := range status.Remotes {
		names = append(names, remote.Name)
	}
	if err := validateRemote(name, target, kind, names); err != nil {
		return nil, err
	}
	return tools.ExternalCommand(p, "gh", "repo", "create", target, "--"+kind, "--source", p.Path, "--remote", name)
}
