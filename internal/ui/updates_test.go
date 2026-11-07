package ui

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

func TestUpdateNoticeMenuAndDismiss(t *testing.T) {
	m := fixture(t)
	m.Update(releaseMsg{version: "0.3.0"})
	if !strings.Contains(plain(m.headerStatus()), "u update 0.3.0") {
		t.Fatal("missing update status")
	}
	m.key(press('u', "u"))
	if m.mode != updateMenu {
		t.Fatal("u must open update menu")
	}
	m.modalKey(press(tea.KeyEscape, ""))
	if m.latestRelease == "" {
		t.Fatal("escape dismissed release")
	}
	m.key(press('u', "u"))
	m.modalKey(press(tea.KeyDown, ""))
	m.modalKey(press(tea.KeyEnter, ""))
	if m.latestRelease != "" || m.mode != home {
		t.Fatal("dismiss did not clear notice")
	}
}
func TestUpdateKeyDoesNotInterruptSearch(t *testing.T) {
	m := fixture(t)
	m.latestRelease = "0.3.0"
	m.searching = true
	m.search.Focus()
	m.key(press('u', "u"))
	if m.mode != home || m.search.Value() != "u" {
		t.Fatal("update key intercepted search")
	}
}

func TestUpdateStatusBeforeActions(t *testing.T) {
	m := fixture(t)
	m.latestRelease = "0.3.0"
	for _, width := range []int{52, 80, 120} {
		m.width, m.height = width, 30
		header := strings.Split(plain(m.homeView()), "\n")[0]
		if strings.Index(header, "u update") < 0 || strings.Index(header, "u update") > strings.Index(header, "Ctrl+K") {
			t.Fatalf("status order at %d: %s", width, header)
		}
	}
}
