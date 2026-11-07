package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"rian/internal/git"
	"rian/internal/project"
	"rian/internal/tools"
)

func (m *Model) startGitHubAdd() tea.Cmd {
	m.mode, m.githubStage, m.formError = githubAdd, 0, ""
	m.githubChoice = 0
	m.query = input("GitHub owner/repository or URL")
	m.resize()
	return nil
}

func (m *Model) githubAddKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "esc" {
		if m.githubStage == 0 {
			m.startAdd()
			return m.query.Focus()
		}
		m.githubStage--
		m.formError = ""
		if m.githubStage == 1 {
			m.query = input("GitHub owner/repository or URL")
			m.query.SetValue(m.githubRepo)
		}
		if m.githubStage == 2 {
			m.query = input("Leave empty to select, or type a location")
			m.query.SetValue(m.githubLocation)
		}
		m.resize()
		return m.query.Focus()
	}
	switch m.githubStage {
	case 0:
		switch key {
		case "up", "j", "left":
			m.githubChoice = 0
			return nil
		case "down", "k", "right":
			m.githubChoice = 1
			return nil
		case "f":
			m.githubChoice = 0
		case "c":
			m.githubChoice = 1
		case "enter", "space", " ":
		default:
			return nil
		}
		if m.githubChoice == 0 {
			if _, err := exec.LookPath("gh"); err != nil {
				m.formError = "Install GitHub CLI and sign in to fork repositories"
				return nil
			}
			m.githubKind = "fork"
		} else {
			m.githubKind = "clone"
		}
		m.githubStage, m.formError = 1, ""
		return m.query.Focus()
	case 1:
		if key == "enter" {
			repo, err := git.NormalizeRepo(strings.TrimSpace(m.query.Value()))
			if err != nil || repo == "" || strings.HasPrefix(repo, "-") {
				m.formError = "Enter a GitHub owner/repository or repository URL"
				return nil
			}
			m.githubRepo, m.githubStage, m.formError = repo, 2, ""
			m.query = input("Leave empty to select, or type a location")
			m.resize()
			return m.query.Focus()
		}
	case 2:
		if key == "enter" {
			if strings.TrimSpace(m.query.Value()) == "" {
				m.busy = true
				return func() tea.Msg { path, err := tools.PickFolder(); return pickedFolderMsg{path: path, err: err} }
			}
			return m.githubChooseLocation(m.query.Value())
		}
	case 3:
		if key == "y" || key == "enter" {
			return m.beginGitHubImport(true)
		}
		if key == "n" {
			return m.beginGitHubImport(false)
		}
		return nil
	}
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	return cmd
}

// Clone into the selected directory itself. Git accepts an empty existing
// directory or a new directory whose parent exists.
func githubDestination(location string) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", fmt.Errorf("enter a destination directory")
	}
	if location == "~" || strings.HasPrefix(location, "~/") || strings.HasPrefix(location, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		location = filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(location, "~"), "/\\"))
	}
	path, err := filepath.Abs(location)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		parent, err := project.CanonicalPath(filepath.Dir(path))
		if err != nil {
			return "", err
		}
		return filepath.Join(parent, filepath.Base(path)), nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("choose a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	if len(entries) != 0 {
		return "", fmt.Errorf("destination is not empty; choose an empty directory")
	}
	return project.CanonicalPath(path)
}
func (m *Model) githubChooseLocation(location string) tea.Cmd {
	path, err := githubDestination(location)
	if err != nil {
		m.formError = err.Error()
		return m.query.Focus()
	}
	m.githubLocation, m.githubPath, m.formError = location, path, ""
	if m.githubKind == "fork" {
		return m.beginGitHubImport(true)
	}
	m.githubStage = 3
	return nil
}

// Each step uses the same output window and only advances after a successful exit.
func (m *Model) githubOutput(title string, prepare func() (*exec.Cmd, error), done func() tea.Cmd) tea.Cmd {
	cmd := m.runAddonOutput(project.Project{ID: "github-import", Path: filepath.Dir(m.githubPath)}, title, prepare)
	m.outputDone = done
	return cmd
}
func (m *Model) beginGitHubImport(keepRemote bool) tea.Cmd {
	repo, destination := m.githubRepo, m.githubPath
	finish := func() tea.Cmd {
		m.mode = add
		m.query = input("Directory path")
		m.query.SetValue(destination)
		return m.inspectDirectory("add", destination, "")
	}
	clone := func(source string) tea.Cmd {
		return m.githubOutput("Clone "+source, func() (*exec.Cmd, error) {
			if _, err := githubDestination(destination); err != nil {
				return nil, err
			}
			var cmd *exec.Cmd
			if _, err := exec.LookPath("gh"); err == nil {
				cmd = exec.Command("gh", "repo", "clone", source, destination)
			} else {
				cmd = exec.Command("git", "clone", "--progress", "https://github.com/"+source+".git", destination)
			}
			cmd.Env = append(cmd.Environ(), "GH_PROMPT_DISABLED=1", "GIT_TERMINAL_PROMPT=0")
			return cmd, nil
		}, func() tea.Cmd {
			if !keepRemote {
				return m.githubOutput("Remove clone remote", func() (*exec.Cmd, error) {
					return tools.GitCommand(project.Project{Path: destination}, "remote", "remove", "origin")
				}, finish)
			}
			if m.githubKind == "fork" {
				status := git.Inspect(destination)
				for _, remote := range status.Remotes {
					if remote.Name == "upstream" {
						return finish()
					}
				}
				return m.githubOutput("Add upstream remote", func() (*exec.Cmd, error) {
					return tools.GitCommand(project.Project{Path: destination}, "remote", "add", "upstream", "https://github.com/"+repo+".git")
				}, finish)
			}
			return finish()
		})
	}
	if m.githubKind == "clone" {
		return clone(repo)
	}
	var fork string
	return m.githubOutput("Fork "+repo, func() (*exec.Cmd, error) {
		user := exec.Command("gh", "api", "user", "--jq", ".login")
		output, err := user.Output()
		if err != nil {
			return nil, fmt.Errorf("sign in with GitHub CLI before forking: %w", err)
		}
		login := strings.TrimSpace(string(output))
		if login == "" || strings.ContainsAny(login, "/\\\r\n ") {
			return nil, fmt.Errorf("invalid GitHub account")
		}
		fork = login + "/" + strings.Split(repo, "/")[1]
		cmd := exec.Command("gh", "repo", "fork", repo, "--clone=false", "--remote=false")
		cmd.Env = append(cmd.Environ(), "GH_PROMPT_DISABLED=1")
		return cmd, nil
	}, func() tea.Cmd { return clone(fork) })
}
func (m *Model) githubAddView() (string, string, string) {
	title := "ADD FROM GITHUB"
	back := keyHint("Esc", "back")
	switch m.githubStage {
	case 0:
		rows := []string{"  Fork", "  Clone"}
		rows[m.githubChoice] = highlight.Render("› " + strings.TrimSpace(rows[m.githubChoice]))
		return title, strings.Join(rows, "\n"), back + "  " + keyHint("↑↓", "choose") + "  " + keyHint("Enter", "select")
	case 1:
		return title + " / " + strings.ToUpper(m.githubKind), m.query.View(), back + "   " + keyHint("Enter", "continue")
	case 2:
		return "LOCATION", m.query.View(), back + "   " + keyHint("Enter", "continue")
	default:
		return "ADD CLONE REMOTE?", oneLine(m.githubRepo) + " as origin\n" + muted.Render(oneLine(m.githubPath)), back + "  " + keyHint("Enter / y", "add") + "  " + keyHint("n", "skip")
	}
}
