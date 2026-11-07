package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"rian/internal/git"
)

var graphColors = []string{"#8ccfe8", "#c9a0ef", "#79d6ac", "#e7be70", "#8c9ee8", "#ed9292"}

func (m *Model) graphRows(s git.Status) []git.GraphLine {
	if m.cfg.Settings.GitGraphScope() == "current" && s.CurrentGraphLines != nil {
		return s.CurrentGraphLines
	}
	return s.GraphLines
}
func (m *Model) selectedGraphHash() string {
	p, ok := m.current()
	if !ok {
		return ""
	}
	rows := m.graphRows(m.statuses[p.ID])
	if m.graphIndex >= 0 && m.graphIndex < len(rows) {
		return rows[m.graphIndex].Hash
	}
	return ""
}
func (m *Model) restoreGraphSelection(hash string) {
	p, ok := m.current()
	if !ok {
		return
	}
	rows := m.graphRows(m.statuses[p.ID])
	m.graphIndex = 0
	for i, row := range rows {
		if hash != "" && row.Hash == hash {
			m.graphIndex = i
			return
		}
	}
	for i, row := range rows {
		if row.Current {
			m.graphIndex = i
			return
		}
	}
	for i, row := range rows {
		if row.Hash != "" {
			m.graphIndex = i
			return
		}
	}
}

func graphPrefix(row git.GraphLine, continuation bool) string {
	rendered := ""
	for _, cell := range row.Cells {
		glyph := cell.Glyph
		if continuation {
			switch glyph {
			case '●':
				glyph = '│'
			case '╱', '╲', '─':
				glyph = ' '
			}
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(graphColors[0]))
		if cell.Color >= 0 && cell.Color < len(graphColors) {
			style = style.Foreground(lipgloss.Color(graphColors[cell.Color]))
		}
		if cell.CurrentBranch {
			style = highlight
		}
		if glyph == '●' {
			glyph = '┼'
			if row.Current {
				glyph = '╋'
				style = highlight.Bold(true)
			}
		}
		rendered += style.Render(string(glyph))
	}
	return rendered
}

func graphRefLabel(ref git.GraphRef) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#79d6ac"))
	label := "local: " + oneLine(ref.Name)
	switch ref.Kind {
	case "remote":
		style = style.Foreground(lipgloss.Color("#8ccfe8"))
		label = "remote: " + oneLine(ref.Name)
	case "tag":
		style = style.Foreground(lipgloss.Color("#e7be70"))
		label = "tag: " + oneLine(ref.Name)
	}
	if ref.Current {
		style = highlight.Bold(true)
		label = "HEAD · " + label
	}
	return style.Render("[" + label + "]")
}

func graphRenderedRows(row git.GraphLine, width int) []string {
	if len(row.Cells) == 0 {
		return []string{fit(textStyle.Render(oneLine(row.Text)), width)}
	}
	prefix := graphPrefix(row, false)
	if row.Hash == "" {
		return []string{fit(prefix, width)}
	}
	hashStyle, subjectStyle := muted, textStyle
	if row.CurrentBranch || row.Current {
		hashStyle, subjectStyle = highlight, highlight
	}
	if row.Current {
		subjectStyle = subjectStyle.Bold(true)
	}
	lines := []string{fit(prefix+hashStyle.Render(row.ShortHash)+" "+subjectStyle.Render(oneLine(row.Subject)), width)}
	if len(row.Refs) == 0 {
		return lines
	}
	indent := graphPrefix(row, true)
	current := indent
	for _, ref := range row.Refs {
		label := graphRefLabel(ref)
		if lipgloss.Width(current) > lipgloss.Width(indent) && lipgloss.Width(current)+1+lipgloss.Width(label) > width {
			lines = append(lines, fit(current, width))
			current = indent
		}
		if lipgloss.Width(current) > lipgloss.Width(indent) {
			current += " "
		}
		if lipgloss.Width(indent)+lipgloss.Width(label) > width {
			if current != indent {
				lines = append(lines, fit(current, width))
				current = indent
			}
			for _, part := range strings.Split(ansi.Hardwrap(label, max(1, width-lipgloss.Width(indent)), false), "\n") {
				lines = append(lines, fit(indent+part, width))
			}
			continue
		}
		current += label
	}
	if current != indent {
		lines = append(lines, fit(current, width))
	}
	return lines
}

func (m *Model) gitGraphView(width, height int) string {
	p, ok := m.current()
	if !ok {
		return ""
	}
	s := m.statuses[p.ID]
	scope := "current"
	if m.cfg.Settings.GitGraphScope() == "all" {
		scope = "all"
	}
	header := []string{titleStyle.Render("GIT GRAPH") + muted.Render(" / "+scope)}
	if s.Branch != "" {
		local := "◉ local: " + oneLine(s.Branch)
		if strings.HasPrefix(s.Branch, "detached") || s.Branch == "unborn" {
			local = "◉ " + oneLine(s.Branch)
		}
		header = append(header, highlight.Bold(true).Render(local))
	}
	if s.Upstream != "" {
		header = append(header, lipgloss.NewStyle().Foreground(lipgloss.Color("#8ccfe8")).Render("↔ remote: "+oneLine(s.Upstream)))
		if s.Ahead+s.Behind > 0 {
			header = append(header, muted.Render(fmt.Sprintf("↑ %d ahead   ↓ %d behind", s.Ahead, s.Behind)))
		}
	} else if s.Branch != "" {
		header = append(header, muted.Render("No tracking remote branch"))
	}
	header = append(header, "")
	rows := m.graphRows(s)
	if len(rows) == 0 {
		header = append(header, muted.Render("No commits yet."))
	} else {
		rendered := []string{}
		selectedEnd := 0
		count := max(1, height-len(header))
		selected := max(0, min(m.graphIndex, len(rows)-1))
		first := max(0, selected-count)
		for i := first; i < min(len(rows), selected+count+1); i++ {
			row := rows[i]
			blockStart := len(rendered)
			block := graphRenderedRows(row, width)
			for _, line := range block {
				if m.graphFocused && i == m.graphIndex {
					style := selectedStyle
					if row.CurrentBranch || row.Current {
						style = style.Foreground(accent)
					}
					line = style.Render(pad(plain(line), width))
				}
				rendered = append(rendered, line)
			}
			if i == m.graphIndex {
				selectedEnd = min(len(rendered)-1, blockStart+count-1)
			}
		}
		start := max(0, min(selectedEnd-count+1, len(rendered)-count))
		header = append(header, rendered[start:min(len(rendered), start+count)]...)
	}
	for i, line := range header {
		header[i] = fit(line, width)
	}
	return strings.Join(header[:min(len(header), max(1, height))], "\n")
}
