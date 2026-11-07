package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"rian/internal/config"
	"rian/internal/detect"
	"rian/internal/git"
	"rian/internal/project"
	"rian/internal/tools"
)

func (m *Model) startAdd() {
	m.autoAddReviewing = false
	m.mode, m.formError = add, ""
	placeholder := "Directory path"
	if _, err := tools.FolderPicker(); err == nil {
		placeholder = "Leave empty to select, or type a path"
	}
	m.query = input(placeholder)
	m.resize()
}

func (m *Model) submitAdd() tea.Cmd {
	if strings.TrimSpace(m.query.Value()) != "" {
		return m.inspectDirectory("add", m.query.Value(), "")
	}
	m.busy, m.formError = true, ""
	return func() tea.Msg {
		path, err := tools.PickFolder()
		return pickedFolderMsg{path: path, err: err}
	}
}

func (m *Model) inspectDirectory(purpose, path, id string) tea.Cmd {
	m.busy = true
	return func() tea.Msg { result, err := detect.Inspect(path); return inspectedMsg{result, err, purpose, id} }
}

func (m *Model) confirmAdd() tea.Cmd {
	p := m.pending.Project
	return m.save(p.ID, "Project added", func(cfg *config.Config) error { return config.Add(cfg, p) })
}

func (m *Model) startEdit(p project.Project) {
	m.mode, m.formError, m.editProject = edit, "", p
	m.fields = nil
	for _, field := range []struct{ label, value, hint string }{
		{"Name", p.Name, "Display name"}, {"Directory", p.Path, "Project directory"},
		{"GitHub", p.GitHub, "owner/repo override (optional)"}, {"Local URL", p.URLs["local"], "http://localhost:3000"}, {"Production URL", p.URLs["production"], "https://example.com"},
	} {
		i := input(field.hint)
		i.SetValue(field.value)
		m.fields = append(m.fields, formField{label: field.label, input: i})
	}
	m.fields = append(m.fields, formField{label: "Pinned", toggle: true, checked: p.Favorite})
	for _, name := range []string{"vscode", "opencode", "kilocode", "terminal"} {
		m.fields = append(m.fields, formField{label: name, toggle: true, checked: p.ToolEnabled(name)})
	}
	m.fieldIndex = 0
	m.resize()
}

func (m *Model) focusField(index int) tea.Cmd {
	m.fields[m.fieldIndex].input.Blur()
	m.fieldIndex = (index + len(m.fields)) % len(m.fields)
	if !m.fields[m.fieldIndex].toggle {
		return m.fields[m.fieldIndex].input.Focus()
	}
	return nil
}

func (m *Model) submitEdit() tea.Cmd {
	if strings.TrimSpace(m.fields[0].input.Value()) == "" {
		m.formError = "Give this project a name"
		return nil
	}
	repo, err := git.NormalizeRepo(strings.TrimSpace(m.fields[2].input.Value()))
	if err != nil {
		m.formError = err.Error()
		return nil
	}
	for _, i := range []int{3, 4} {
		if err := tools.ValidateURL(strings.TrimSpace(m.fields[i].input.Value())); err != nil {
			m.formError = m.fields[i].label + ": " + err.Error()
			return nil
		}
	}
	p := m.editProject
	p.Name = strings.TrimSpace(m.fields[0].input.Value())
	p.GitHub = repo
	p.URLs = map[string]string{"local": strings.TrimSpace(m.fields[3].input.Value()), "production": strings.TrimSpace(m.fields[4].input.Value())}
	p.Favorite = m.fields[5].checked
	p.Tools = map[string]bool{}
	for _, field := range m.fields[6:] {
		p.Tools[field.label] = field.checked
	}
	m.editProject = p
	return m.inspectDirectory("edit", m.fields[1].input.Value(), p.ID)
}

func (m *Model) startCustom(name, command string) {
	m.commandOriginal = name
	m.mode, m.commandEditing, m.formError = prompt, true, ""
	m.promptAction, m.commandField = "custom", 0
	m.commandName, m.commandValue = input("e.g. dev, deploy, docs"), input("e.g. npm run dev")
	m.commandName.SetValue(name)
	m.commandValue.SetValue(command)
	m.resize()
}

func (m *Model) submitCustom() tea.Cmd {
	name, value := strings.TrimSpace(m.commandName.Value()), strings.TrimSpace(m.commandValue.Value())
	if name == "" || value == "" {
		m.formError = "A name and command are required"
		return nil
	}
	p, ok := m.current()
	if !ok {
		return nil
	}
	original := m.commandOriginal
	return m.save(p.ID, "Command saved", func(cfg *config.Config) error {
		for i := range cfg.Projects {
			if cfg.Projects[i].ID == p.ID {
				if cfg.Projects[i].Commands == nil {
					cfg.Projects[i].Commands = map[string]string{}
				}
				if original != "" && original != name {
					delete(cfg.Projects[i].Commands, original)
				}
				cfg.Projects[i].Commands[name] = value
				return nil
			}
		}
		return fmt.Errorf("project no longer exists")
	})
}

func (m *Model) commandEdit(remove bool) tea.Cmd {
	items := m.filteredActions()
	if len(items) == 0 {
		return nil
	}
	item := items[min(m.menuIndex, len(items)-1)]
	if item.ID != "run" {
		return nil
	}
	p, ok := m.current()
	if !ok {
		return nil
	}
	if !remove {
		m.startCustom(item.Value, p.AllCommands()[item.Value])
		return m.commandName.Focus()
	}
	if _, custom := p.Commands[item.Value]; !custom {
		return m.notify("Detected command", fmt.Errorf("detected commands come from project files; Ctrl+E adds an override"))
	}
	return m.save(p.ID, "Custom command removed", func(cfg *config.Config) error {
		for i := range cfg.Projects {
			if cfg.Projects[i].ID == p.ID {
				delete(cfg.Projects[i].Commands, item.Value)
				return nil
			}
		}
		return fmt.Errorf("project no longer exists")
	})
}
