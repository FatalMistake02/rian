package addon

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"rian/internal/project"
)

func TestImportValidation(t *testing.T) {
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", `{"format_version":1,"version":"1.0.0","id":"test","name":"Test","actions":[{"id":"open","name":"Open","kind":"url","url":"https://example.com"}]}`, true},
		{"format", `{"format_version":2,"version":"1.0.0","id":"test","name":"Test","actions":[]}`, false},
		{"unknown", `{"format_version":1,"version":"1.0.0","id":"test","name":"Test","script":"bad","actions":[]}`, false},
		{"trailing", `{"format_version":1,"version":"1.0.0"} {}`, false},
		{"escaping", `{"format_version":1,"version":"1.0.0","id":"test","name":"Test","detect":{"files":["../secret"]},"actions":[{"id":"open","name":"Open","kind":"url","url":"https://example.com"}]}`, false},
		{"setting", `{"format_version":1,"version":"1.0.0","id":"test","name":"Test","settings":[{"id":"flag","name":"Flag","type":"bool","default":"maybe"}],"actions":[{"id":"open","name":"Open","kind":"url","url":"https://example.com"}]}`, false},
		{"template", `{"format_version":1,"version":"1.0.0","id":"test","name":"Test","actions":[{"id":"open","name":"Open","kind":"cli","executable":"echo","args":["${setting.missing}"]}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "addon.json")
			if err := os.WriteFile(path, []byte(test.body), 0600); err != nil {
				t.Fatal(err)
			}
			item, err := Read(path)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
			if test.valid && (!item.Enabled || item.Source != path) {
				t.Fatal("import metadata missing")
			}
		})
	}
}
func TestDetectionAndLiteralArguments(t *testing.T) {
	root := t.TempDir()
	p := project.New("Test", root)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The test executable is on PATH, but detection must never start it.
	t.Setenv("PATH", filepath.Dir(binary))
	name := filepath.Base(binary)
	item := Installed{Enabled: true, Definition: Definition{FormatVersion: 1, Version: "1.0.0", ID: "test", Name: "Test", Detect: Detection{Files: []string{"package*.json"}, Tools: []string{name}}, Settings: []Setting{{ID: "text", Name: "Text", Type: "text", Default: "default"}}, Actions: []Action{{ID: "run", Name: "Run", Kind: "command", Executable: name, Args: []string{"${setting.text}", "${project.path}"}}}}, Values: map[string]string{"text": "a b; $(touch marker) ${project.path}"}}
	if len(item.Actions(p)) != 0 {
		t.Fatal("matched missing file")
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if len(item.Actions(p)) != 1 {
		t.Fatal("did not detect file/tool")
	}
	cmd, err := item.Command(item.Definition.Actions[0], p)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Dir != root || cmd.Args[1] != item.Values["text"] || cmd.Args[2] != root {
		t.Fatalf("arguments changed: %v", cmd.Args)
	}
	if _, err := os.Stat(filepath.Join(root, "marker")); !os.IsNotExist(err) {
		t.Fatal("detection executed command")
	}
	item.Enabled = false
	if len(item.Actions(p)) != 0 {
		t.Fatal("disabled addon offered actions")
	}
	item.Enabled = true
	item.Definition.Actions[0].Detect.Tools = []string{"rian-nonexistent-tool"}
	if len(item.Actions(p)) != 0 {
		t.Fatal("missing action tool was offered")
	}
}
func TestCloneAndSettingTypes(t *testing.T) {
	item := Installed{Definition: Definition{Settings: []Setting{{ID: "build", Name: "Build", Type: "choice", Default: "yes", Options: []string{"yes", "no"}}}}, Values: map[string]string{"build": "no"}}
	copy := item.Clone()
	copy.Values["build"] = "yes"
	copy.Definition.Settings[0].Options[0] = "changed"
	if item.Values["build"] != "no" || item.Definition.Settings[0].Options[0] != "yes" {
		t.Fatal("clone shared mutable values")
	}
	for _, bad := range []string{"NaN", "+Inf", "-Inf", "words"} {
		if (Setting{Name: "Number", Type: "number"}).ValidateValue(bad) == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
func TestExampleManifests(t *testing.T) {
	for _, name := range []string{"template", "git", "npm"} {
		if _, err := Read(filepath.Join("..", "..", "examples", "addons", name+".json")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGitExampleDetectsRepositoryRoots(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	example, err := Read(filepath.Join("..", "..", "examples", "addons", "git.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"directory", "worktree file"} {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			p := project.New("Repository", root)
			item := example.Clone()
			if actions := item.Actions(p); len(actions) != 1 || actions[0].ID != "init" {
				t.Fatalf("non-repositories should offer initialization only: %v", actions)
			}
			if marker == "directory" {
				err = os.Mkdir(filepath.Join(root, ".git"), 0700)
			} else {
				err = os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../repository/.git/worktrees/example\n"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			actions := item.Actions(p)
			if len(actions) < 11 {
				t.Fatalf("repository actions missing: %v", actions)
			}
			if value := item.Value(item.Definition.Settings[0]); value != "20" {
				t.Fatalf("numeric default changed: %q", value)
			}
			item.Values = map[string]string{"history-limit": "7"}
			if err := item.Validate(); err != nil {
				t.Fatal(err)
			}
			for _, action := range actions {
				if action.ID != "history" {
					continue
				}
				cmd, err := item.Command(action, p)
				if err != nil {
					t.Fatal(err)
				}
				want := []string{"--no-pager", "log", "--oneline", "--decorate", "-n", "7"}
				if cmd.Dir != root || !slices.Equal(cmd.Args[1:], want) {
					t.Fatalf("history setting not passed literally: %v in %s", cmd.Args, cmd.Dir)
				}
			}
			t.Setenv("PATH", t.TempDir())
			if len(item.Actions(p)) != 0 {
				t.Fatal("offered Git actions without Git installed")
			}
		})
	}
}

func TestGitExampleWorkflow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	item, err := Read(filepath.Join("..", "..", "examples", "addons", "git.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	p := project.New("Repository", root)
	actionCommand := func(id string) *exec.Cmd {
		t.Helper()
		for _, action := range item.Actions(p) {
			if action.ID == id {
				cmd, err := item.Command(action, p)
				if err != nil {
					t.Fatal(err)
				}
				return cmd
			}
		}
		t.Fatalf("action %q is unavailable", id)
		return nil
	}
	runAction := func(id string) {
		t.Helper()
		if output, err := actionCommand(id).CombinedOutput(); err != nil {
			t.Fatalf("action %s: %s (%v)", id, output, err)
		}
	}
	runGit := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s (%v)", args, output, err)
		}
		return strings.TrimSpace(string(output))
	}
	runAction("init")
	runGit("symbolic-ref", "HEAD", "refs/heads/main")
	if err := os.WriteFile(filepath.Join(root, "sample.txt"), []byte("example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runAction("stage-all")
	if staged := runGit("diff", "--cached", "--name-only"); staged != "sample.txt" {
		t.Fatalf("stage action missed project changes: %q", staged)
	}
	runGit("-c", "user.name=Rian Test", "-c", "user.email=rian@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "Example initial commit")
	runAction("create-branch")
	if branch := runGit("branch", "--show-current"); branch != "feature/example" {
		t.Fatalf("new branch setting was ignored: %q", branch)
	}
	runAction("switch")
	if branch := runGit("branch", "--show-current"); branch != "main" {
		t.Fatalf("switch branch setting was ignored: %q", branch)
	}
	for _, c := range []struct{ setting, action string }{{"branch", "switch"}, {"new-branch", "create-branch"}} {
		item.Values[c.setting] = "--detach"
		if output, err := actionCommand(c.action).CombinedOutput(); err == nil {
			t.Fatalf("setting was accepted as a branch option: %s", output)
		}
		if branch := runGit("branch", "--show-current"); branch != "main" {
			t.Fatalf("setting changed HEAD as an option: %q", branch)
		}
	}
}

func TestPageManifestValidation(t *testing.T) {
	for _, body := range []string{
		`{"format_version":1,"version":"1.0.0","id":"notes","name":"Notes","pages":[{"id":"notes","name":"Notes","text":"Useful notes"}]}`,
		`{"format_version":1,"version":"1.0.0","id":"test","name":"Test","pages":[{"id":"tools","name":"Tools","sections":[{"title":"Links","actions":["open"]}]}],"actions":[{"id":"open","name":"Open","kind":"url","url":"https://example.com"}]}`,
	} {
		path := filepath.Join(t.TempDir(), "addon.json")
		_ = os.WriteFile(path, []byte(body), 0600)
		if _, err := Read(path); err != nil {
			t.Fatal(err)
		}
	}
	d := Definition{FormatVersion: 1, Version: "1.0.0", ID: "test", Name: "Test", Pages: []Page{{ID: "tools", Name: "Tools", Actions: []string{"missing"}}}}
	if d.Validate() == nil {
		t.Fatal("page accepted missing action")
	}
	d.Pages[0].Actions = nil
	d.Pages = append(d.Pages, d.Pages[0])
	if d.Validate() == nil {
		t.Fatal("duplicate page ids accepted")
	}
}
