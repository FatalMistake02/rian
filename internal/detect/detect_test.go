package detect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestInspectStacksAndCommands(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string
		stack          []string
		command, value string
	}{
		{"next-pnpm", map[string]string{"package.json": `{"scripts":{"dev":"next dev","test":"vitest"},"dependencies":{"next":"15"}}`, "pnpm-lock.yaml": ""}, []string{"Node.js", "pnpm", "Next.js"}, "dev", "pnpm run dev"},
		{"bun-expo", map[string]string{"package.json": `{"packageManager":"bun@1.2","scripts":{"start":"expo start"},"dependencies":{"expo":"1","react-native":"1"}}`}, []string{"Node.js", "bun", "Expo", "React Native"}, "start", "bun run start"},
		{"go", map[string]string{"go.mod": "module example.test\n"}, []string{"Go"}, "test", "go test ./..."},
		{"rust", map[string]string{"Cargo.toml": "[package]\n"}, []string{"Rust"}, "build", "cargo build"},
		{"python", map[string]string{"requirements.txt": "pytest", "main.py": ""}, []string{"Python"}, "run", ""},
		{"flutter", map[string]string{"pubspec.yaml": "dependencies:\n  flutter:\n    sdk: flutter\n"}, []string{"Flutter"}, "test", "flutter test"},
		{"vite-yarn", map[string]string{"package.json": `{"scripts":{"build":"vite build"},"devDependencies":{"vite":"6"}}`, "yarn.lock": ""}, []string{"Node.js", "yarn", "Vite"}, "build", "yarn run build"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := Inspect(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, stack := range tt.stack {
				if !slices.Contains(result.Project.Stack, stack) {
					t.Errorf("missing %s in %v", stack, result.Project.Stack)
				}
			}
			command := result.Project.AutoCommands[tt.command]
			if command == "" || tt.value != "" && command != tt.value {
				t.Fatalf("command: %q, want %q", command, tt.value)
			}
		})
	}
}

func TestInvalidFilesAndScriptNames(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{broken"), 0600)
	result, err := Inspect(dir)
	if err != nil || len(result.Warnings) == 0 {
		t.Fatalf("malformed manifest must be nonfatal: %+v %v", result, err)
	}
	commands := ScriptCommands("npm", map[string]string{"test:unit": "vitest", "dev && bad": "ignored", "": "ignored", "lint": "eslint", "--help": "ignored", "-s": "ignored"})
	if len(commands) != 2 || commands["test:unit"] != "npm run test:unit" {
		t.Fatalf("unsafe or missing scripts: %v", commands)
	}
	if actual := PackageManager("yarn@4", func(string) bool { return true }); actual != "yarn" {
		t.Fatalf("declared manager should win: %s", actual)
	}
	if actual := PackageManager("", func(s string) bool { return s == "bun.lockb" }); actual != "bun" {
		t.Fatal(actual)
	}
	if _, err := Inspect(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
}

func TestGoRunUsesExecutableDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "server"), 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "library.go"), []byte("package server\n"), 0600)
	os.WriteFile(filepath.Join(dir, "cmd", "server", "main.go"), []byte("package main\nfunc main() {}\n"), 0600)
	if commands := GoRunCommands(dir); commands["run"] != "go run ./cmd/server" || len(commands) != 1 {
		t.Fatalf("bad cmd/ detection: %v", commands)
	}
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0600)
	if commands := GoRunCommands(dir); commands["run"] != "go run ." {
		t.Fatalf("bad root detection: %v", commands)
	}
}
