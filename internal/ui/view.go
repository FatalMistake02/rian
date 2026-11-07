package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"rian/internal/detect"
	"rian/internal/project"
	"rian/internal/tools"
)

func (m *Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Starting Rian")
		v.AltScreen = true
		v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
		return v
	}
	var content string
	if m.width < 52 || m.height < 16 {
		content = fit(highlight.Bold(true).Render("RIAN")+"\n\n"+muted.Render("Resize to at least 52 × 16.\nYour projects are ready when you are.\n\nq quit · Ctrl+K actions"), m.width)
	} else {
		content = m.homeView()
		if m.mode != home {
			modal := m.modalView()
			x, y := max(0, (m.width-lipgloss.Width(modal))/2), max(0, (m.height-lipgloss.Height(modal))/2)
			content = lipgloss.NewCompositor(lipgloss.NewLayer(content), lipgloss.NewLayer(modal).X(x).Y(y).Z(1)).Render()
		}
	}
	if status := m.bottomStatus(); status != "" && m.height >= 16 && m.width >= 52 {
		x := max(2, m.width-2-lipgloss.Width(status))
		content = lipgloss.NewCompositor(lipgloss.NewLayer(content), lipgloss.NewLayer(status).X(x).Y(m.height-1).Z(2)).Render()
	}

	v := tea.NewView(content)
	v.AltScreen, v.ReportFocus, v.WindowTitle = true, true, "Rian · Projects"
	if m.overview {
		v.WindowTitle = "Rian · Overview"
	}
	if m.projectFull {
		if p, ok := m.current(); ok {
			v.WindowTitle = "Rian · " + oneLine(p.Name)
		}
	}
	v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	v.BackgroundColor, v.ForegroundColor = background, foreground
	return v
}

func (m *Model) homeView() string {
	w, h := m.width-4, m.height-2
	head := highlight.Bold(true).Render("RIAN")
	minimumHeadWidth := 4
	if m.projectFull {
		if _, ok := m.current(); ok {
			minimumHeadWidth = 8
		}
	}
	right := keyHint("Ctrl+K", "actions")
	// Keep notices discoverable before allocating the optional project title.
	if status := m.headerStatusWidth(max(0, w-2-minimumHeadWidth-lipgloss.Width(right)-4)); status != "" {
		right = status + "   " + right
	}
	if m.projectFull {
		if p, ok := m.current(); ok {
			nameWidth := max(1, w-2-lipgloss.Width(right)-1-7)
			head += muted.Render(" / ") + titleStyle.Render(fit(oneLine(p.Name), nameWidth))
		}
	}
	header := "   " + head + strings.Repeat(" ", max(1, w-2-lipgloss.Width(head)-lipgloss.Width(right))) + right
	var body string
	pinLabel := "pin"
	if p, ok := m.current(); ok && p.Favorite {
		pinLabel = "unpin"
	}
	footer := keyHint("Esc", "overview") + "   " + keyHint("↑↓ / jk", "navigate") + "   " + keyHint("Enter", "open project") + "   " + keyHint("a", "add") + "   " + keyHint("p", pinLabel) + "   " + keyHint("Delete", "remove") + "   " + keyHint("/", "search") + "   " + keyHint("s", "settings") + "   " + keyHint("q", "quit")
	if m.overview {
		leftW := min(30, max(20, w*28/100))
		rightW := w - leftW - 1
		body = lipgloss.JoinHorizontal(lipgloss.Top, box(m.overviewMenu(leftW-4, h-2), leftW, h, m.mode == home && !m.overviewFocused), " ", box(m.overviewView(rightW-4, h-2), rightW, h, m.overviewFocused))
		footer = keyHint("↑↓ / jk", "navigate") + "   " + keyHint("Enter / Space", "select") + "   " + keyHint("a", "add") + "   " + keyHint("q", "quit")
		if _, ok := m.selectedAddonPage(); ok {
			if m.overviewFocused {
				footer = keyHint("Esc / ←", "menu") + "   " + keyHint("↑↓ / jk", "choose action") + "   " + keyHint("Enter", "run action")
			} else {
				footer = keyHint("↑↓ / jk", "navigate") + "   " + keyHint("→", "page") + "   " + keyHint("Enter / Space", "select") + "   " + keyHint("a", "add") + "   " + keyHint("q", "quit")
			}
		} else if alerts, _ := m.overviewAlerts(); len(alerts) > 0 {
			if m.overviewFocused {
				footer = keyHint("Esc / ←", "menu") + "   " + keyHint("↑↓ / jk", "choose") + "   " + keyHint("Enter", "open project")
			} else {
				footer = keyHint("↑↓ / jk", "navigate") + "   " + keyHint("→", "attention") + "   " + keyHint("Enter / Space", "select") + "   " + keyHint("a", "add") + "   " + keyHint("q", "quit")
			}
		}
	} else if len(m.cfg.Projects) == 0 {
		body = box(m.welcomeView(w-4, h-2), w, h, true)
		footer = keyHint("Esc", "overview") + "   " + keyHint("a", "add project") + "   " + keyHint("s", "settings") + "   " + keyHint("q", "quit")
	} else if m.projectFull {
		p, ok := m.current()
		if ok && w >= 44 {
			leftW := max(23, w*45/100)
			rightW := w - leftW - 1
			m.resetNotesScroll()
			rightPanel := box(scrolledProjectNotesView(p, rightW-4, h-2, m.notesOffset), rightW, h, m.notesFocused)
			if m.statuses[p.ID].Repository {
				notesH := max(5, min(10, h/3))
				graphH := h - notesH - 1
				rightPanel = lipgloss.JoinVertical(lipgloss.Left,
					box(m.gitGraphView(rightW-4, graphH-2), rightW, graphH, m.graphFocused),
					"",
					box(scrolledProjectNotesView(p, rightW-4, notesH-2, m.notesOffset), rightW, notesH, m.notesFocused))
			}
			body = lipgloss.JoinHorizontal(lipgloss.Top, box(m.detailView(leftW-4, h-2), leftW, h, !m.graphFocused && !m.notesFocused), " ", rightPanel)
		} else {
			body = box(m.detailView(w-4, h-2), w, h, true)
		}
		footer = keyHint("Esc", "projects") + "   " + keyHint("p", pinLabel) + "   " + keyHint("Delete", "remove") + "   " + keyHint("s", "settings") + "   " + keyHint("q", "quit")
		if ok && m.statuses[p.ID].Repository {
			if m.graphFocused {
				footer = keyHint("Esc / ←", "project") + "   " + keyHint("↑↓ / jk", "commit")
				if _, err := m.graphCommitURL(); err == nil {
					footer += "   " + keyHint("Enter", "open on GitHub")
				}
				footer += "   " + keyHint("b", "branches")
			} else {
				footer = keyHint("Esc", "projects") + "   " + keyHint("→", "Git graph") + "   " + keyHint("p", pinLabel) + "   " + keyHint("Delete", "remove") + "   " + keyHint("s", "settings") + "   " + keyHint("q", "quit")
			}
		}
		if m.notesFocused {
			footer = keyHint("Esc / ←", "project") + "   " + keyHint("↑↓ / jk", "scroll notes") + "   " + keyHint("n", "edit notes")
		} else if !m.graphFocused {
			footer = strings.Replace(footer, "   "+keyHint("p", pinLabel), "   "+keyHint("↓", "notes")+"   "+keyHint("p", pinLabel), 1)
		}
	} else {
		leftW := min(44, max(25, w*36/100))
		if m.width < 84 {
			leftW = max(23, w/2-1)
		}
		rightW := w - leftW - 1
		left := box(m.listView(leftW-4, h-2), leftW, h, m.mode == home)
		rightPanel := box(m.detailView(rightW-4, h-2), rightW, h, false)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", rightPanel)
	}
	footerWidth := w - 2
	if status := m.bottomStatus(); status != "" {
		footerWidth = max(0, footerWidth-lipgloss.Width(status)-1)
	}
	return header + "\n" + lipgloss.NewStyle().MarginLeft(2).Render(body) + "\n   " + fit(footer, footerWidth)

}

// Transient status and validation share the bottom-right notification area.
func (m *Model) bottomStatus() string {
	message, style := m.toast, muted
	if m.toastError {
		style = warning
	}
	if m.formError != "" && m.mode != home {
		message, style = m.formError, warning
	}
	if message == "" && m.busy {
		message = m.spin.View() + muted.Render(" Working")
	} else {
		message = style.Render(fit(oneLine(message), min(80, m.width-6)))
	}
	if message == "" {
		return ""
	}
	return lipgloss.NewStyle().Background(background).Padding(0, 1).Render(message)
}

// A simple five-row alphabet keeps the large welcome title readable.
func largeText(text string) string {
	glyphs := map[rune][5]string{
		'W': {"█   █", "█   █", "█ █ █", "█ █ █", " █ █ "},
		'E': {"███", "█  ", "███", "█  ", "███"},
		'L': {"█  ", "█  ", "█  ", "█  ", "███"},
		'C': {"███", "█  ", "█  ", "█  ", "███"},
		'O': {"███", "█ █", "█ █", "█ █", "███"},
		'M': {"█   █", "██ ██", "█ █ █", "█   █", "█   █"},
		'T': {"███", " █ ", " █ ", " █ ", " █ "},
		'R': {"██ ", "█ █", "██ ", "█ █", "█ █"},
		'I': {"███", " █ ", " █ ", " █ ", "███"},
		'A': {"███", "█ █", "███", "█ █", "█ █"},
		'N': {"█   █", "██  █", "█ █ █", "█  ██", "█   █"},
		' ': {"  ", "  ", "  ", "  ", "  "},
	}
	rows := make([]string, 5)
	for _, letter := range text {
		for row, glyph := range glyphs[letter] {
			if rows[row] != "" {
				rows[row] += " "
			}
			rows[row] += glyph
		}
	}
	return strings.Join(rows, "\n")
}

func (m *Model) welcomeView(width, height int) string {
	title := highlight.Render(largeText("WELCOME TO RIAN"))
	if lipgloss.Width(title) > width {
		if height >= 13 {
			title = highlight.Render(largeText("WELCOME TO") + "\n\n" + largeText("RIAN"))
		} else {
			title = titleStyle.Render("Welcome to") + "\n" + highlight.Render(largeText("RIAN"))
		}
	}
	lines := strings.Split(title+"\n\n"+keyHint("a", "add your first project"), "\n")
	for i, line := range lines {
		lines[i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, line)
	}
	return lipgloss.PlaceVertical(height, lipgloss.Center, strings.Join(lines, "\n"))
}

func (m *Model) listView(width, height int) string {
	projects := m.projects()
	heading := titleStyle.Render("PROJECTS")
	if m.searching || m.search.Value() != "" {
		heading += "  " + highlight.Render("/ ") + m.search.View()
	}
	lines := []string{pad(heading, width), ""}
	if len(projects) == 0 {
		if m.search.Value() != "" {
			lines = append(lines, muted.Render("No matching projects."), "", muted.Render("Esc clears your search."))
		} else {
			lines = append(lines, "", titleStyle.Render("A home for your work."), "", fit(muted.Render("Save a directory once."), width), fit(muted.Render("Pick up where you left off."), width), "", keyHint("a", "Add your first project"))
		}
		return strings.Join(lines, "\n")
	}
	visible := max(1, (height-len(lines)-2)/3)
	start := max(0, m.selected-visible+1)
	end := min(len(projects), start+visible)
	for i := start; i < end; i++ {
		p := projects[i]
		prefix := "  "
		if i == m.selected {
			prefix = "› "
		}
		name := oneLine(p.Name)
		if p.Favorite {
			name = "★ " + name
		}
		status := m.statuses[p.ID]
		branch := fit(oneLine(status.Branch), min(11, width/3))
		marker := ""
		if status.Repository {
			if status.Dirty() {
				marker = highlight.Render(" ●")
			} else {
				marker = highlight.Render(" ○")
			}
		}
		if m.loading[p.ID] {
			marker = " " + m.spin.View()
		}
		if marker != "" {
			marker += " "
		}
		meta := muted.Render(branch) + marker
		nameWidth := max(4, width-lipgloss.Width(meta)-3)
		row := pad(prefix+fit(name, nameWidth-2), width-lipgloss.Width(meta)) + meta
		if i == m.selected {
			row = selectedStyle.Render(pad(prefix+fit(name, nameWidth-2), width-lipgloss.Width(meta))) +
				selectedStyle.Foreground(mutedColor).Render(branch) +
				selectedStyle.Foreground(accent).Render(plain(marker))
		} else {
			row = textStyle.Render(row)
		}
		lines = append(lines, row, muted.Render(pad("  "+shortPath(p.Path), width)), "")
	}
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	position := fmt.Sprintf("%d of %d", min(m.selected+1, len(projects)), len(projects))
	if len(projects) > visible {
		position += " · scroll ↑↓"
	}
	lines = append(lines, muted.Render(fit(position, width)))
	return strings.Join(lines, "\n")
}

func (m *Model) detailView(width, height int) string {
	p, ok := m.current()
	if !ok {
		return strings.Join([]string{titleStyle.Render("READY WHEN YOU ARE"), "", "", highlight.Render("Local projects. Less friction."), "", fit(muted.Render("Open your editor, coding tools, and commands"), width), fit(muted.Render("from one keyboard-first workspace."), width), "", keyHint("a", "add a project"), keyHint("Ctrl+K", "browse actions")}, "\n")
	}
	s := m.statuses[p.ID]
	lines := []string{titleStyle.Render(fit(oneLine(p.Name), width)), muted.Render(fit(shortPath(p.Path), width)), ""}
	if len(p.Stack) > 0 {
		lines = append(lines, muted.Render(fit(strings.Join(p.Stack, " · "), width)), "")
	}
	label := func(name, value string) string {
		return muted.Render(pad(name, min(14, width/3))) + textStyle.Render(fit(oneLine(value), max(1, width-min(14, width/3))))
	}
	repo := p.GitHub
	if repo == "" {
		repo = s.GitHub
	}
	if repo != "" {
		lines = append(lines, highlight.Render("GITHUB"), textStyle.Render(fit(oneLine(repo), width)), "")
	}
	if s.Repository {
		state := good.Render("○ clean")
		if s.Dirty() {
			state = warning.Render("● changes")
		}
		lines = append(lines, label("Branch", s.Branch), label("Status", "")+state, label("Modified", fmt.Sprint(s.Modified)), label("Untracked", fmt.Sprint(s.Untracked)), label("Ahead / back", fmt.Sprintf("↑ %d   ↓ %d", s.Ahead, s.Behind)), "")
		if s.Commit != "" {
			lines = append(lines, muted.Render("LAST COMMIT"), fit(oneLine(s.Commit), width), muted.Render(relativeTime(s.CommitTime)), "")
		}
		if s.Remote != "" && repo == "" {
			lines = append(lines, label("Remote", s.Remote))
		}
	} else if m.loading[p.ID] {
		lines = append(lines, m.spin.View()+muted.Render(" Reading Git status"), "")
	} else {
		lines = append(lines, muted.Render("No Git repository detected."), "")
	}
	if s.Error != "" {
		lines = append(lines, fit(warning.Render(s.Error), width))
	}
	if m.projectFull {
		commands := m.visibleCommands(p)
		if len(commands) > 0 {
			lines = append(lines, highlight.Render("COMMANDS"))
			for _, name := range detect.CommandNames(commands) {
				lines = append(lines, fit(textStyle.Render(name)+muted.Render("  "+oneLine(commands[name])), width))
			}
			lines = append(lines, "")
		}
	}
	// Keep the action hints anchored below the details at every terminal size.
	hints := m.projectHints(p, width)
	space := max(2, height-len(hints)-1)
	noteLines := []string{}
	if !m.projectFull && strings.TrimSpace(p.Notes) != "" {
		noteHeight := min(space, max(2, space-len(lines)))
		if noteHeight >= 2 {
			noteLines = strings.Split(projectNotesView(p, width, noteHeight), "\n")
		}
	}
	detailSpace := max(0, space-len(noteLines))
	if len(lines) > detailSpace {
		lines = lines[:detailSpace]
	}
	lines = append(lines, noteLines...)
	for len(lines) < space {
		lines = append(lines, "")
	}
	lines = append(lines, muted.Render(strings.Repeat("─", width)))
	for _, hint := range hints {
		lines = append(lines, fit(hint, width))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) projectHints(p project.Project, width int) []string {
	available := map[string]bool{}
	for _, action := range m.actions(p) {
		if action.Disabled == "" && m.actionShown(action) {
			available[action.ID] = true
		}
	}
	_, githubErr := tools.GitHubURL(p, m.statuses[p.ID])
	available["github"] = githubErr == nil && m.shownFor(p, "github")
	available["commands"] = true // This menu also lets the user add a command.
	available["gitMenu"] = m.gitMenuAvailable(p) && m.shownFor(p, "git")
	items := []struct{ id, key, label string }{
		{"commands", "r", "Commands"},
		{"files", "f", "Files"}, {"gitMenu", "G", "Git"}, {"github", "g", "GitHub"},
		{"kilocode", "K", "KiloCode"}, {"notes", "n", "Notes"}, {"opencode", "o", "OpenCode"},
		{"terminal", "t", "Terminal"}, {"vscode", "v", "VS Code"},
	}

	for _, action := range m.addonActions(p) {
		id := strings.TrimPrefix(action.ID, "addon/")
		if key := m.addonKey(id); key != "" {
			items = append(items, struct{ id, key, label string }{action.ID, key, oneLine(action.Label)})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].label) < strings.ToLower(items[j].label) })
	var lines []string
	row := ""
	for _, item := range items {
		if !available[item.id] {
			continue
		}
		hint := keyHint(item.key, item.label)
		if row != "" && lipgloss.Width(row)+3+lipgloss.Width(hint) > width {
			lines = append(lines, row)
			row = ""
		}
		if row != "" {
			row += "   "
		}
		row += hint
	}
	if row != "" {
		lines = append(lines, row)
	}
	return lines
}

func (m *Model) modalView() string {
	w := max(20, min(78, m.width-8))
	inner := w - 6
	var body, title, footer string
	switch m.mode {
	case launchChoice:
		title = "OPEN IN"
		body = m.launchChoiceView(inner)
		footer = keyHint("↑↓", "choose") + "  " + keyHint("Enter", "open") + "  " + keyHint("Esc", "cancel")
	case addonImport, addonEdit, addonRemove:
		title, body, footer = m.addonFormView()
	case remoteForm, remoteConfirm:
		title, body, footer = m.remoteFormView()
	case settings:
		title = m.settingsHeader(inner)
		body = m.settingsView(inner)
		footer = m.settingsFooter()
	case notes:
		title, body, footer = m.notesView()
	case updateMenu:
		title, body, footer = m.updateMenuView()
	case addonUpdates, addonUpdateConfirm:
		title, body, footer = m.addonUpdatesView(inner)
	case autoAddReview:
		title, body, footer = m.autoAddReviewView(inner)
	case autoAddLocation:
		title, body = "AUTO ADD / LOCATION", m.query.View()
		footer = keyHint("Esc", "back") + "  " + keyHint("Enter", "save") + "\n" + keyHint("Ctrl+C", "cancel settings")
	case githubAdd:
		title, body, footer = m.githubAddView()
	case add:
		title = "ADD PROJECT"
		body = m.query.View()
		footer = keyHint("Enter", "continue") + "   " + keyHint("Esc", "cancel")
		if m.query.Value() == "" {
			footer = keyHint("Enter", "add") + "  " + keyHint("g", "add from GitHub") + "  " + keyHint("Esc", "cancel")
		}
	case addConfirm:
		title = "PROJECT FOUND"
		body = m.viewport.View()
		footer = keyHint("↑↓", "scroll") + "   " + keyHint("Enter", "save project") + "   " + keyHint("Esc", "cancel")
	case edit:
		title = "EDIT PROJECT"
		lines := []string{muted.Render("Commands: press r, then Ctrl+N / Ctrl+E."), ""}
		visible := max(3, min(len(m.fields), m.height-13))
		start := max(0, m.fieldIndex-visible+1)
		for i := start; i < min(len(m.fields), start+visible); i++ {
			field := m.fields[i]
			label := muted.Render(pad(field.label, 16))
			value := field.input.View()
			if field.toggle {
				value = muted.Render("[ ] disabled")
				if field.checked {
					value = good.Render("[✓] enabled")
				}
				if field.label == "Pinned" {
					value = "[ ]"
					if field.checked {
						value = highlight.Render("[★]")
					}
				}
			}
			prefix := "  "
			if i == m.fieldIndex {
				prefix = highlight.Render("› ")
				label = textStyle.Render(pad(field.label, 16))
			}
			lines = append(lines, fit(prefix+label+value, inner))
		}
		body = strings.Join(lines, "\n")
		footer = keyHint("Tab / ↑↓", "field") + "  " + keyHint("Space", "toggle") + "  " + keyHint("Ctrl+S", "save")
		if inner < 54 {
			footer = keyHint("Tab", "field") + "  " + keyHint("Space", "toggle") + "  " + keyHint("Ctrl+S", "save")
		}
		footer += "\n" + keyHint("Esc", "cancel")
	case palette, commands, gitMenu, branchMenu, addonMenu, remoteMenu:
		title = "ACTIONS"
		if m.mode == remoteMenu {
			title = "ADD REMOTE"
		}
		if m.mode == addonMenu {
			title = oneLine(m.addonTitle)
		}
		if m.mode == commands {
			title = "PROJECT COMMANDS"
		}
		if m.mode == gitMenu {
			title = "GIT ACTIONS"
		}
		if m.mode == branchMenu {
			title = "SWITCH BRANCH"
		}
		items := m.filteredActions()
		if m.mode == palette {
			body = m.query.View() + "\n" + muted.Render(strings.Repeat("─", inner)) + "\n" + m.paletteView(items, inner, max(2, m.height-13))
			footer = keyHint("Esc", "close") + "   " + keyHint("↑↓", "choose") + "   " + keyHint("Enter", "select")
			break
		}
		lines := []string{m.query.View(), muted.Render(strings.Repeat("─", inner)), ""}
		bodyCapacity := m.height - 11
		if m.mode == commands {
			bodyCapacity--
		}
		stride := 2
		if m.height < 24 || m.mode == gitMenu || m.mode == branchMenu || m.mode == remoteMenu {
			stride = 1
		}
		visible := max(1, min(8, (bodyCapacity-3)/stride))
		start := max(0, m.menuIndex-visible+1)
		if len(items) == 0 {
			lines = append(lines, muted.Render("No matching actions."))
		}
		for i := start; i < min(len(items), start+visible); i++ {
			item := items[i]
			prefix := "  "
			if i == m.menuIndex {
				prefix = "› "
			}
			row := pad(prefix+oneLine(item.Label), inner)
			if i == m.menuIndex {
				row = selectedStyle.Render(row)
			} else if item.Disabled != "" {
				row = muted.Render(row)
			}
			detail := item.Detail
			if item.Disabled != "" {
				detail = item.Disabled
			}
			lines = append(lines, row)
			if stride == 2 {
				lines = append(lines, muted.Render(fit("  "+oneLine(detail), inner)))
			}
		}
		body = strings.Join(lines, "\n")
		footer = keyHint("Esc", "close") + "   " + keyHint("↑↓", "choose") + "   " + keyHint("Enter", "select")
		if m.mode == commands {
			footer += "\n" + keyHint("Ctrl+N", "new") + "  " + keyHint("Ctrl+E", "edit") + "  " + keyHint("Ctrl+D", "remove")
		}
	case gitOutput:
		title = "GIT / " + oneLine(m.gitTitle)
		if m.outputPrepare != nil {
			title = oneLine(m.gitTitle)
		}
		body = m.viewport.View()
		footer = keyHint("↑↓", "scroll")
		if m.gitRunning {
			footer += "  " + m.spin.View() + muted.Render(" running")
		} else {
			footer = keyHint("Esc", "close") + "  " + footer
		}
	case stageConfirm:
		title = "STAGE FILES BEFORE COMMIT?"
		body = "No files are staged.\n\nStage all changed and new files?"
		footer = keyHint("Enter / y", "stage and continue") + "  " + keyHint("Esc / n", "cancel")
	case removeConfirm:
		title = "REMOVE PROJECT?"
		body = titleStyle.Render(fit(oneLine(m.editProject.Name), inner))
		footer = keyHint("Enter / y", "remove") + "   " + keyHint("Esc / n", "keep")
	case quitConfirm:
		title = "COMMANDS ARE STILL RUNNING"
		body = fmt.Sprintf("%d active process(es).\n\nQuitting will stop their process trees.", m.manager.Active())
		footer = keyHint("Enter / y", "stop and quit") + "   " + keyHint("Esc / n", "stay")
	case prompt:
		title = "RUN COMMAND"
		if m.promptAction == "commit" {
			title = "COMMIT"
		}
		if m.promptAction == "newBranch" {
			title = "CREATE BRANCH"
		}
		body = m.query.View()
		footer = keyHint("Enter", "continue") + "   " + keyHint("Esc", "cancel")
		if m.commandEditing {
			title = "CUSTOM COMMAND"
			body = muted.Render("Name (an existing name overrides detection)") + "\n" + m.commandName.View() + "\n\n" + muted.Render("Shell command, run in the project directory") + "\n" + m.commandValue.View()
			footer = keyHint("Tab", "field") + "   " + keyHint("Ctrl+S", "save") + "   " + keyHint("Esc", "cancel")
		}
	case processView:
		title = "PROCESS OUTPUT"
		s := m.processState
		state := muted.Render("STOPPED")
		if s.Error != "" {
			state = warning.Render("FAILED")
		}
		if s.Running {
			state = good.Render("● RUNNING")
		}
		if s.Command == "" {
			state = muted.Render("No command has run yet.")
		}
		meta := state
		if s.PID > 0 {
			meta += muted.Render(fmt.Sprintf("   PID %d   started %s", s.PID, s.Started.Local().Format("15:04:05")))
		}
		body = fit(oneLine(s.Command), inner) + "\n" + fit(meta, inner) + "\n\n" + m.viewport.View()
		if s.Error != "" {
			body += "\n" + warning.Render(fit(oneLine(s.Error), inner))
		}
		follow := "paused"
		if m.followOutput {
			follow = "live"
		}
		footer = keyHint("Esc", "back") + "   " + keyHint("f", "follow: "+follow) + "   " + keyHint("r", "rerun") + "   " + keyHint("s", "stop")
		if inner < 60 {
			footer = keyHint("Esc", "back") + "  " + keyHint("f", "follow") + "  " + keyHint("r", "rerun") + "  " + keyHint("s", "stop")
		}
	}

	bodyLines := strings.Split(body, "\n")
	footerLines := strings.Split(footer, "\n")
	allowed := max(1, m.height-10-len(footerLines))
	if len(bodyLines) > allowed {
		bodyLines = bodyLines[:allowed]
	}
	for i := range bodyLines {
		bodyLines[i] = fit(bodyLines[i], inner)
	}
	for i := range footerLines {
		footerLines[i] = fit(footerLines[i], inner)
	}
	content := titleStyle.Render(title) + "\n\n" + strings.Join(bodyLines, "\n") + "\n\n" + strings.Join(footerLines, "\n")
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).
		Background(background).Foreground(foreground).Padding(1, 2).Width(w).
		MaxWidth(w).MaxHeight(m.height - 2).Render(content)
}

func (m *Model) detectionSummary(result detect.Result) string {
	p := result.Project
	lines := []string{titleStyle.Render(oneLine(p.Name)), muted.Render(oneLine(p.Path)), ""}
	if len(p.Stack) > 0 {
		lines = append(lines, "Stack    "+strings.Join(p.Stack, " · "))
	}
	if result.Git.Repository {
		lines = append(lines, "Branch   "+oneLine(result.Git.Branch))
	}
	if result.Git.GitHub != "" {
		lines = append(lines, "GitHub   "+oneLine(result.Git.GitHub))
	}
	lines = append(lines, "", highlight.Render("DETECTED COMMANDS"))
	if len(p.AutoCommands) == 0 {
		lines = append(lines, muted.Render("None yet. Add custom commands with r."))
	}
	for _, name := range detect.CommandNames(p.AutoCommands) {
		lines = append(lines, pad(oneLine(name), 14)+oneLine(p.AutoCommands[name]))
	}
	lines = append(lines, "", highlight.Render("TOOLS"))
	for _, tool := range []string{"code", "opencode", "kilocode", "gh", "lazygit"} {
		status := "not in PATH"
		if result.Available[tool] {
			status = "available"
		}
		lines = append(lines, pad(tool, 14)+status)
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "", oneLine(warning))
	}
	return strings.Join(lines, "\n")
}

func shortPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if project.SamePath(path, home) {
			return "~"
		}
		if len(path) > len(home) && project.SamePath(path[:len(home)], home) && (path[len(home)] == '/' || path[len(home)] == '\\') {
			return "~" + oneLine(path[len(home):])
		}
	}
	return oneLine(path)
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hr ago", int(d.Hours()))
	}
	if d < 30*24*time.Hour {
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
	return t.Format("Jan 2, 2006")
}
