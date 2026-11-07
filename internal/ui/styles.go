package ui

import (
	"fmt"
	"rian/internal/config"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent        = lipgloss.Color("#fa11b0")
	background    = lipgloss.Color("#101015")
	foreground    = lipgloss.Color("#e9e6ef")
	mutedColor    = lipgloss.Color("#898591")
	borderColor   = lipgloss.Color("#37333f")
	textStyle     = lipgloss.NewStyle().Foreground(foreground)
	muted         = lipgloss.NewStyle().Foreground(mutedColor)
	highlight     = lipgloss.NewStyle().Foreground(accent)
	titleStyle    = textStyle.Bold(true)
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("#2e182a")).Foreground(foreground)
	good          = lipgloss.NewStyle().Foreground(lipgloss.Color("#9bceb0"))
	warning       = lipgloss.NewStyle().Foreground(lipgloss.Color("#dfb882"))
)

func input(placeholder string) textinput.Model {
	i := textinput.New()
	i.Prompt = ""
	i.Placeholder = placeholder
	i.CharLimit = 4096
	i.SetWidth(50)
	s := i.Styles()
	s.Focused.Prompt, s.Focused.Text = highlight, textStyle
	s.Focused.Placeholder, s.Blurred.Placeholder = muted, muted
	s.Cursor.Color = accent
	i.SetStyles(s)
	return i
}

// External output is rendered as plain text: OSC hyperlinks, cursor controls,
// and escape sequences must never be allowed to manipulate the Rian screen.
func plain(value string) string {
	value = ansi.Strip(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func oneLine(value string) string { return strings.ReplaceAll(plain(value), "\n", " ") }

func fit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "")
}

func pad(value string, width int) string {
	value = fit(value, width)
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}

func box(content string, width, height int, active bool) string {
	color := borderColor
	if active {
		color = accent
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Width(max(1, width)).Height(max(1, height)).Padding(0, 1).
		MaxWidth(width).MaxHeight(height).Render(content)
}

func keyHint(key, action string) string {
	keys := strings.Split(key, " / ")
	for i, value := range keys {
		keys[i] = highlight.Render(value)
	}
	return strings.Join(keys, muted.Render(" / ")) + muted.Render(" "+action)
}

func (m *Model) applyAppearance() {
	color := m.effectiveAccent(m.cfg.Settings)
	accent = lipgloss.Color(color.Hex)
	highlight = lipgloss.NewStyle().Foreground(accent)
	rgb, _ := strconv.ParseUint(strings.TrimPrefix(color.Hex, "#"), 16, 32)
	tint := fmt.Sprintf("#%02x%02x%02x", (16*4+int(rgb>>16))/5, (16*4+int(rgb>>8&255))/5, (21*4+int(rgb&255))/5)
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color(tint)).Foreground(foreground)
	styleInput := func(i *textinput.Model) {
		s := i.Styles()
		s.Focused.Prompt = highlight
		s.Cursor.Color = accent
		i.SetStyles(s)
	}
	styleInput(&m.settingsSearch)
	styleInput(&m.search)
	styleInput(&m.query)
	styleInput(&m.commandName)
	styleInput(&m.commandValue)
	for i := range m.fields {
		styleInput(&m.fields[i].input)
	}
	m.spin.Style = highlight
	if m.mode == notes {
		m.styleNotesInput()
	}
}

func (m *Model) effectiveAccent(settings config.Settings) config.AccentColor {
	color := settings.AccentColor()
	if color.ID == "random" {
		if m.randomAccent.Hex != "" {
			return m.randomAccent
		}
		return config.AccentColors[0]
	}
	return color
}
