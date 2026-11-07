package ui

import (
	"crypto/rand"
	"log"
	"os"
	"os/exec"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"rian/internal/config"
	"rian/internal/detect"
	"rian/internal/git"
	"rian/internal/process"
	"rian/internal/project"
	"rian/internal/tools"
)

type mode int

const (
	home mode = iota
	add
	addConfirm
	edit
	palette
	commands
	gitMenu
	branchMenu
	stageConfirm
	gitOutput
	removeConfirm
	quitConfirm
	prompt
	processView
	settings
	launchChoice
	addonMenu
	addonImport
	addonEdit
	addonRemove
	remoteMenu
	remoteForm
	remoteConfirm
	githubAdd
	autoAddReview
	autoAddLocation
	notes
	updateMenu
	addonUpdates
	addonUpdateConfirm
)

type Action struct{ ID, Label, Detail, Value, Disabled string }
type formField struct {
	label   string
	input   textinput.Model
	toggle  bool
	checked bool
}

type Model struct {
	version, latestRelease                             string
	updating                                           bool
	addonUpdatesAvailable                              map[string]addonUpdate
	addonUpdatesChecked                                map[string]addonUpdateCheckState
	addonUpdatesDismissed                              map[string]string
	addonUpdatesChecking                               bool
	addonUpdateID                                      string
	addonUpdateChoice                                  int
	addonUpdateCheck                                   addonChecker
	notesFocused                                       bool
	notesOffset                                        int
	notesScrollProject                                 string
	notesProject                                       project.Project
	notesInput                                         textarea.Model
	autoAddKey, autoAddScanError                       string
	autoAddCandidates                                  []project.Project
	autoAddLastScan                                    time.Time
	autoAddScanning, autoAddBlink, autoAddReviewing    bool
	autoAddReviewIndex                                 int
	githubChoice                                       int
	githubStage                                        int
	githubKind, githubRepo, githubLocation, githubPath string
	outputDone                                         func() tea.Cmd
	remoteKind                                         string
	remoteNames                                        []string
	remoteProject                                      project.Project
	remoteName                                         string
	remoteTarget                                       string
	remoteInputs                                       [2]textinput.Model
	remoteField                                        int
	addonRow                                           settingRow
	addonTitle                                         string
	addonLoading                                       bool
	outputPrepare                                      func() (*exec.Cmd, error)
	overview                                           bool
	overviewIndex                                      int
	overviewFocused                                    bool
	overviewAlertIndex                                 int
	randomAccent                                       config.AccentColor
	projectFull                                        bool
	graphFocused                                       bool
	graphIndex                                         int
	gitJobID                                           string
	gitProject                                         project.Project
	gitTitle                                           string
	gitSteps                                           [][]string
	gitStep                                            int
	gitLog                                             string
	gitRunning                                         bool
	gitJobStarted                                      bool
	gitError                                           string
	startupDir                                         string
	settingsOriginal                                   config.Settings
	settingsChanged                                    bool
	pendingLaunch                                      func(string) tea.Cmd
	launchModes                                        []string
	launchIndex                                        int
	launchTitle                                        string
	settingsPage                                       string
	settingsSearch                                     textinput.Model
	settingsSearching                                  bool
	settingsDraft                                      config.Settings
	settingsIndex                                      int
	settingsRows                                       []settingRow
	store                                              *config.Store
	manager                                            *process.Manager
	launcher                                           tools.System
	logger                                             *log.Logger
	cfg                                                config.Config
	width, height                                      int
	mode                                               mode
	selected                                           int
	search                                             textinput.Model
	searching                                          bool
	query                                              textinput.Model
	menuIndex                                          int
	menu                                               []Action
	available                                          map[string]bool
	statuses                                           map[string]git.Status
	loading                                            map[string]bool
	lastRefresh                                        map[string]time.Time
	spin                                               spinner.Model
	viewport                                           viewport.Model
	toast                                              string
	toastError                                         bool
	toastID                                            int
	busy                                               bool
	pending                                            detect.Result
	fields                                             []formField
	fieldIndex                                         int
	editProject                                        project.Project
	promptAction                                       string
	commitAutoStage                                    bool
	commandName                                        textinput.Model
	commandValue                                       textinput.Model
	commandEditing                                     bool
	commandField                                       int
	commandOriginal                                    string
	formError                                          string
	processID                                          string
	processState                                       process.Snapshot
	followOutput                                       bool
}

type readyMsg struct {
	cfg       config.Config
	available map[string]bool
	err       error
}
type pickedFolderMsg struct {
	path string
	err  error
}

type inspectedMsg struct {
	result  detect.Result
	err     error
	purpose string
	id      string
}
type gitMsg struct {
	id     string
	status git.Status
}
type savedMsg struct {
	cfg        config.Config
	id, notice string
	err        error
}
type actionMsg struct {
	id, notice string
	err        error
	refresh    bool
}
type processMsg struct {
	id    string
	state process.Snapshot
	err   error
}
type tickMsg time.Time
type toastExpired int

func New(store *config.Store, manager *process.Manager, logger *log.Logger, versions ...string) *Model {
	version := ""
	if len(versions) > 0 {
		version = versions[0]
	}
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(highlight))
	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(12))
	vp.KeyMap.Up.SetKeys("up", "j")
	vp.KeyMap.Down.SetKeys("down", "k")
	startupDir, _ := os.Getwd()
	var choice [1]byte
	_, _ = rand.Read(choice[:])
	randomAccent := config.AccentColors[int(choice[0])%(len(config.AccentColors)-1)]
	return &Model{version: version, overview: true, randomAccent: randomAccent, startupDir: startupDir, store: store, manager: manager, logger: logger, search: input("Search projects"), query: input(""),
		spin: s, viewport: vp,
		statuses: map[string]git.Status{}, loading: map[string]bool{}, lastRefresh: map[string]time.Time{}, available: map[string]bool{}, followOutput: true}
}

func (m *Model) Init() tea.Cmd {
	store := m.store
	return tea.Batch(func() tea.Msg {
		cfg, err := store.Update(func(*config.Config) error { return nil })
		return readyMsg{cfg, detect.AvailableTools(), err}
	}, checkRelease(m.version), tick(), m.spin.Tick)
}

func tick() tea.Cmd { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func (m *Model) projects() []project.Project { return project.Filter(m.cfg.Projects, m.search.Value()) }

func (m *Model) current() (project.Project, bool) {
	projects := m.projects()
	if len(projects) == 0 {
		return project.Project{}, false
	}
	return projects[min(m.selected, len(projects)-1)], true
}

func (m *Model) selectID(id string) {
	for i, p := range m.projects() {
		if p.ID == id {
			m.selected = i
			return
		}
	}
	m.selected = min(m.selected, max(0, len(m.projects())-1))
}

func (m *Model) refresh(p project.Project, force bool) tea.Cmd {
	if m.loading[p.ID] || !force && time.Since(m.lastRefresh[p.ID]) < 15*time.Second {
		return nil
	}
	m.loading[p.ID] = true
	return func() tea.Msg { return gitMsg{p.ID, git.Inspect(p.Path)} }
}

func (m *Model) notify(message string, err error) tea.Cmd {
	m.toastID++
	id := m.toastID
	m.toast, m.toastError = message, err != nil
	if err != nil {
		m.toast = err.Error()
		if m.logger != nil {
			m.logger.Printf("%s: %v", message, err)
		}
	}
	return tea.Tick(7*time.Second, func(time.Time) tea.Msg { return toastExpired(id) })
}

func (m *Model) save(id, notice string, change func(*config.Config) error) tea.Cmd {
	m.busy = true
	store := m.store
	return func() tea.Msg { cfg, err := store.Update(change); return savedMsg{cfg, id, notice, err} }
}

func (m *Model) closeModal() {
	m.settingsSearch.Blur()
	m.settingsSearching = false
	m.mode = home
	m.pendingLaunch = nil
	m.query.Blur()
	m.formError = ""
	m.commandEditing = false
}

func (m *Model) quit() tea.Cmd {
	if m.manager.Active() > 0 {
		m.mode = quitConfirm
		return nil
	}
	return tea.Quit
}

func (m *Model) resize() {
	if m.mode == notes {
		m.notesInput.SetWidth(max(5, min(78, m.width-8)-6))
		m.notesInput.SetHeight(max(3, min(18, m.height-12)))
	}
	for i := range m.remoteInputs {
		m.remoteInputs[i].SetWidth(max(5, min(78, m.width-8)-8))
	}
	w := max(10, min(78, m.width-8))
	m.query.SetWidth(w - 7)
	leftWidth := min(44, max(25, (m.width-4)*36/100))
	if m.width < 84 {
		leftWidth = max(23, (m.width-4)/2-1)
	}
	m.search.SetWidth(max(1, min(42, leftWidth-16)))
	for i := range m.fields {
		m.fields[i].input.SetWidth(max(5, w-25))
	}
	m.commandName.SetWidth(max(5, w-8))
	m.commandValue.SetWidth(max(5, w-8))
	m.viewport.SetWidth(max(5, w-6))
	viewHeight := m.height - 11
	if m.mode == processView {
		viewHeight -= 3
	}
	m.viewport.SetHeight(max(1, min(18, viewHeight)))
	m.viewport.SoftWrap = true
}
