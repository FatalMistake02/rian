// Package detect inspects a handful of files in the project root. It never
// installs packages, evaluates project files, or walks node_modules.
package detect

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"rian/internal/git"
	"rian/internal/project"
	"rian/internal/tools"
)

type Result struct {
	Project   project.Project
	Git       git.Status
	Available map[string]bool
	Warnings  []string
}

type PackageJSON struct {
	Name            string                     `json:"name"`
	PackageManager  string                     `json:"packageManager"`
	Scripts         map[string]string          `json:"scripts"`
	Dependencies    map[string]json.RawMessage `json:"dependencies"`
	DevDependencies map[string]json.RawMessage `json:"devDependencies"`
}

func AvailableTools() map[string]bool {
	result := map[string]bool{}
	for _, name := range []string{"code", "opencode", "kilo", "kilocode", "gh", "lazygit", "git", "npm", "pnpm", "yarn", "bun", "go", "cargo", "python", "python3", "flutter"} {
		_, err := tools.LookPath(name)
		result[name] = err == nil
	}
	result["files"] = tools.FilesAvailable()
	result["kilocode"] = result["kilo"] || result["kilocode"]
	return result
}

func Inspect(path string) (Result, error) {
	canonical, err := project.CanonicalPath(path)
	if err != nil {
		return Result{}, err
	}
	p := project.New(filepath.Base(canonical), canonical)
	p.AutoCommands = map[string]string{}
	result := Result{Project: p, Available: AvailableTools()}
	exists := func(name string) bool {
		info, err := os.Stat(filepath.Join(canonical, name))
		return err == nil && !info.IsDir()
	}
	addStack := func(name string) { result.Project.Stack = append(result.Project.Stack, name) }
	commands := result.Project.AutoCommands
	if exists("package.json") {
		var pkg PackageJSON
		data, err := readSmall(filepath.Join(canonical, "package.json"))
		if err == nil {
			err = json.Unmarshal(data, &pkg)
		}
		if err != nil {
			result.Warnings = append(result.Warnings, "Could not read package.json: "+err.Error())
		} else {
			addStack("Node.js")
			manager := PackageManager(pkg.PackageManager, exists)
			addStack(manager)
			for name, command := range ScriptCommands(manager, pkg.Scripts) {
				commands[name] = command
			}
			for _, framework := range []struct{ dep, name string }{{"next", "Next.js"}, {"vite", "Vite"}, {"expo", "Expo"}, {"react-native", "React Native"}, {"react", "React"}} {
				_, prod := pkg.Dependencies[framework.dep]
				_, dev := pkg.DevDependencies[framework.dep]
				if prod || dev {
					addStack(framework.name)
				}
			}
		}
	}
	if exists("go.mod") {
		addStack("Go")
		defaults(commands, map[string]string{"build": "go build ./...", "test": "go test ./..."})
		defaults(commands, GoRunCommands(canonical))
	}
	if exists("Cargo.toml") {
		addStack("Rust")
		defaults(commands, map[string]string{"run": "cargo run", "build": "cargo build", "test": "cargo test", "lint": "cargo clippy"})
	}
	if exists("pyproject.toml") || exists("requirements.txt") || exists("setup.py") || exists("Pipfile") {
		addStack("Python")
		interpreter := "python3"
		if runtime.GOOS == "windows" {
			interpreter = "python"
		}
		defaults(commands, map[string]string{"test": interpreter + " -m pytest"})
		if exists("manage.py") {
			defaults(commands, map[string]string{"dev": interpreter + " manage.py runserver"})
		}
		if exists("main.py") {
			defaults(commands, map[string]string{"run": interpreter + " main.py"})
		}
	}
	if exists("pubspec.yaml") {
		data, _ := readSmall(filepath.Join(canonical, "pubspec.yaml"))
		if strings.Contains(string(data), "sdk: flutter") {
			addStack("Flutter")
			defaults(commands, map[string]string{"run": "flutter run", "build": "flutter build bundle", "test": "flutter test", "lint": "flutter analyze"})
		}
	}
	result.Git = git.Inspect(canonical)
	return result, nil
}

func readSmall(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 2*1024*1024+1))
	if len(data) > 2*1024*1024 {
		return nil, fmt.Errorf("file is larger than 2 MB")
	}
	return data, err
}

func defaults(commands, values map[string]string) {
	for key, value := range values {
		if _, exists := commands[key]; !exists {
			commands[key] = value
		}
	}
}

func PackageManager(declared string, exists func(string) bool) string {
	manager := strings.SplitN(declared, "@", 2)[0]
	if manager == "npm" || manager == "pnpm" || manager == "yarn" || manager == "bun" {
		return manager
	}
	for _, candidate := range []struct {
		name  string
		files []string
	}{{"bun", []string{"bun.lock", "bun.lockb"}}, {"pnpm", []string{"pnpm-lock.yaml"}}, {"yarn", []string{"yarn.lock"}}} {
		for _, file := range candidate.files {
			if exists(file) {
				return candidate.name
			}
		}
	}
	return "npm"
}

func ScriptCommands(manager string, scripts map[string]string) map[string]string {
	result := map[string]string{}
	for name := range scripts {
		// npm script names outside this portable set can be added as a custom
		// command. Exclude CLI options as well as shell metacharacters.
		if name == "" || strings.HasPrefix(name, "-") {
			continue
		}
		safe := true
		for _, r := range name {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_:-.", r)) {
				safe = false
				break
			}
		}
		if safe {
			result[name] = manager + " run " + name
		}
	}
	return result
}

func CommandNames(commands map[string]string) []string {
	var names []string
	for name := range commands {
		names = append(names, name)
	}
	order := map[string]int{"dev": 1, "run": 2, "start": 3, "build": 4, "test": 5, "lint": 6, "typecheck": 7}
	sort.Slice(names, func(i, j int) bool {
		a, b := order[names[i]], order[names[j]]
		if a == 0 {
			a = 100
		}
		if b == 0 {
			b = 100
		}
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	return names
}

// Go repositories often keep their executable under cmd/ and their root is a
// library. Inspect package clauses so the suggested run command is usable.
func GoRunCommands(root string) map[string]string {
	result := map[string]string{}
	if hasMainPackage(root) {
		result["run"] = "go run ."
		return result
	}
	entries, _ := os.ReadDir(filepath.Join(root, "cmd"))
	var runnable []string
	for i, entry := range entries {
		if i >= 64 {
			break
		}
		if entry.IsDir() && hasMainPackage(filepath.Join(root, "cmd", entry.Name())) {
			// Only construct a shell command from portable directory names.
			if _, safe := ScriptCommands("go", map[string]string{entry.Name(): ""})[entry.Name()]; safe {
				runnable = append(runnable, entry.Name())
			}
		}
	}
	for _, name := range runnable {
		result["run:"+name] = "go run ./cmd/" + name
	}
	if len(runnable) == 1 {
		result["run"] = result["run:"+runnable[0]]
		delete(result, "run:"+runnable[0])
	}
	return result
}

func hasMainPackage(dir string) bool {
	entries, _ := os.ReadDir(dir)
	for i, entry := range entries {
		if i >= 256 {
			break
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := readSmall(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), data, parser.PackageClauseOnly)
		if err == nil && file.Name.Name == "main" {
			return true
		}
	}
	return false
}
