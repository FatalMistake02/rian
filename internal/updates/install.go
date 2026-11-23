package updates

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"rian/internal/project"
	"rian/internal/tools"
	"strings"
	"time"
)

const Repository = "FatalMistake02/rian"
const Formula = "fatalmistake02/rian/rian"

// Set by official binary builds. Source builds must be updated from source.
var distribution = "source"

type Installation struct {
	Method     string `json:"method"`
	Executable string `json:"executable"`
}

func output(name string, args ...string) (string, error) {
	cmd, err := tools.ExternalCommand(project.Project{Path: os.TempDir()}, name, args...)
	if err != nil {
		return "", err
	}
	var data bytes.Buffer
	cmd.Stdout = &data
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return "", err
	}
	timer := time.AfterFunc(8*time.Second, func() { _ = cmd.Process.Kill() })
	defer timer.Stop()
	err = cmd.Wait()
	return strings.TrimSpace(data.String()), err
}

func samePath(a, b string) bool {
	x, e1 := os.Stat(a)
	y, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(x, y)
}

func Detect() (Installation, error) {
	exe, err := os.Executable()
	if err != nil {
		return Installation{}, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return Installation{}, err
	}
	return detect(exe, output), nil
}

func detect(exe string, query func(string, ...string) (string, error)) Installation {
	i := Installation{Executable: exe, Method: "source"}
	// The package root is fixed relative to dist/binaries/<target>/rian.
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(exe))))
	if filepath.Base(filepath.Dir(filepath.Dir(exe))) == "binaries" && filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(exe)))) == "dist" {
		if global, err := query("bun", "pm", "ls", "-g"); err == nil {
			// Bun prints its configured global directory, including custom bunfig paths.
			first := strings.SplitN(global, "\n", 2)[0]
			if dir, _, ok := strings.Cut(first, " node_modules ("); ok && samePath(root, filepath.Join(dir, "node_modules", "@fatalmistake02", "rian")) {
				i.Method = "bun"
				return i
			}
		}
		if global, err := query("npm", "root", "-g"); err == nil && samePath(root, filepath.Join(global, "@fatalmistake02", "rian")) {
			i.Method = "npm"
			return i
		}
		i.Method = "package" // Local installs and npx/bunx caches are never modified.
		return i
	}
	if strings.Contains(filepath.ToSlash(exe), "/Cellar/rian/") {
		if prefix, err := query("brew", "--prefix", Formula); err == nil && samePath(filepath.Dir(filepath.Dir(exe)), prefix) {
			i.Method = "brew"
			return i
		}
		i.Method = "package"
		return i
	}
	if distribution == "standalone" {
		i.Method = "standalone"
	}
	return i
}

func releaseEndpoint() string {
	i, err := Detect()
	if err == nil && (i.Method == "standalone" || i.Method == "brew") {
		return "https://api.github.com/repos/" + Repository + "/releases/latest"
	}
	return "https://registry.npmjs.org/@fatalmistake02%2frian/latest"
}

// Command refuses to update a different installation than the running executable.
func Command(version string) (*exec.Cmd, error) {
	if _, ok := parts(version); !ok {
		return nil, fmt.Errorf("invalid release version")
	}
	i, err := Detect()
	if err != nil {
		return nil, err
	}
	return i.command(version)
}

func (i Installation) command(version string) (*exec.Cmd, error) {
	p := project.Project{Path: os.TempDir()}
	switch i.Method {
	case "npm":
		return tools.ExternalCommand(p, "npm", "install", "-g", Package+"@"+strings.TrimPrefix(version, "v"))
	case "bun":
		return tools.ExternalCommand(p, "bun", "add", "-g", Package+"@"+strings.TrimPrefix(version, "v"))
	case "brew":
		// Run through Rian so brew update and upgrade execute sequentially without a shell.
		return exec.Command(i.Executable, "--update-homebrew", version), nil
	case "standalone":
		return exec.Command(i.Executable, "--update-standalone", version), nil
	default:
		return nil, fmt.Errorf("this installation is not managed by npm, Bun, Homebrew, or the standalone installer; update it using its original installation method")
	}
}

func UpdateHomebrew(version string) error {
	if _, ok := parts(version); !ok {
		return fmt.Errorf("invalid release version")
	}
	i, err := Detect()
	if err != nil {
		return err
	}
	if i.Method != "brew" {
		return fmt.Errorf("not the active Homebrew installation")
	}
	for _, args := range [][]string{{"update"}, {"upgrade", Formula}} {
		cmd, err := tools.ExternalCommand(project.Project{Path: os.TempDir()}, "brew", args...)
		if err != nil {
			return err
		}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	prefix, err := output("brew", "--prefix", Formula)
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(prefix, "bin", "rian"), "--version")
	data, err := cmd.Output()
	if err != nil {
		return err
	}
	got := strings.TrimSpace(strings.TrimPrefix(string(data), "rian "))
	if Newer(version, got) {
		return fmt.Errorf("Homebrew has not published %s yet; try again after the tap updates", version)
	}
	return nil
}
