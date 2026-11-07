// Package updates checks the public npm release and updates global npm installs.
package updates

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"rian/internal/project"
	"rian/internal/tools"
	"strconv"
	"strings"
	"time"
)

const Package = "@fatalmistake02/rian"

func parts(v string) ([3]int, bool) {
	var out [3]int
	p := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(p) != 3 {
		return out, false
	}
	for i, s := range p {
		if s == "" || len(s) > 1 && s[0] == '0' {
			return out, false
		}
		for _, digit := range s {
			if digit < '0' || digit > '9' {
				return out, false
			}
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
func Newer(latest, current string) bool {
	a, ok := parts(latest)
	if !ok {
		return false
	}
	b, ok := parts(current)
	if !ok {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
func Check(current string) (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://registry.npmjs.org/@fatalmistake02%2frian/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry: %s", resp.Status)
	}
	var release struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return "", err
	}
	if Newer(release.Version, current) {
		return release.Version, nil
	}
	return "", nil
}

// Command refuses to update a different installation than the running executable.
func Command(version string) (*exec.Cmd, error) {
	if _, ok := parts(version); !ok {
		return nil, fmt.Errorf("invalid release version")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, err
	}
	rootCmd, err := tools.ExternalCommand(project.Project{Path: os.TempDir()}, "npm", "root", "-g")
	if err != nil {
		return nil, err
	}
	output, err := rootCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("npm is unavailable; update with npm install -g %s@%s", Package, version)
	}
	root, err := filepath.EvalSymlinks(filepath.Join(strings.TrimSpace(string(output)), "@fatalmistake02", "rian"))
	if err != nil || !strings.HasPrefix(exe, root+string(os.PathSeparator)+"dist"+string(os.PathSeparator)+"binaries"+string(os.PathSeparator)) {
		return nil, fmt.Errorf("this is not the active global npm installation; update your installation manually with npm install -g %s@%s", Package, version)
	}
	return tools.ExternalCommand(project.Project{Path: os.TempDir()}, "npm", "install", "-g", Package+"@"+version)
}
