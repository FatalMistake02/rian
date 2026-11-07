// Package config stores local settings with atomic writes and a cross-process
// lock. Updates always reload the file so two Rian instances do not lose edits.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"rian/internal/project"
)

const Version = 1

type Config struct {
	AutoAddSkipped map[string]bool   `json:"auto_add_skipped,omitempty"`
	Settings       Settings          `json:"settings,omitempty"`
	Version        int               `json:"version"`
	Revision       uint64            `json:"revision,omitempty"`
	Projects       []project.Project `json:"projects"`
}

type Store struct {
	Path string
	mu   sync.Mutex
}

func DefaultPath() (string, error) {
	if dir := os.Getenv("RIAN_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rian", "config.json"), nil
}

func New(path string) *Store { return &Store{Path: path} }

func (s *Store) Load() (Config, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Version: Version, Projects: []project.Project{}}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("invalid config %s (file preserved): %w", s.Path, err)
	}
	if cfg.Version != Version {
		return Config{}, fmt.Errorf("unsupported config version %d (file preserved)", cfg.Version)
	}
	if cfg.Settings.Accent == "electric-blue" {
		cfg.Settings.Accent = "glacial-blue"
	}
	for id, mode := range cfg.Settings.Launch {
		if mode == "tab" {
			cfg.Settings.Launch[id] = "window"
		}
	}
	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("invalid config (file preserved): %w", err)
	}
	project.Sort(cfg.Projects)
	return cfg, nil
}

func validate(cfg Config) error {
	if err := cfg.Settings.validate(); err != nil {
		return err
	}
	ids := map[string]bool{}
	for i, p := range cfg.Projects {
		if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Name) == "" || !filepath.IsAbs(p.Path) {
			return fmt.Errorf("project %d needs an ID, name, and absolute path", i+1)
		}
		if ids[p.ID] {
			return fmt.Errorf("duplicate project ID %s", p.ID)
		}
		ids[p.ID] = true
		for _, prior := range cfg.Projects[:i] {
			if project.SamePath(p.Path, prior.Path) {
				return fmt.Errorf("directory already belongs to %s", prior.Name)
			}
		}
	}
	return nil
}

func (s *Store) Update(change func(*Config) error) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Config{}, err
	}
	lock := s.Path + ".lock"
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := os.Mkdir(lock, 0700)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return Config{}, err
		}
		if time.Now().After(deadline) {
			return Config{}, fmt.Errorf("config is locked by another Rian instance; if none is running, remove %s", lock)
		}
		time.Sleep(30 * time.Millisecond)
	}
	defer os.Remove(lock)
	cfg, err := s.Load()
	if err != nil {
		return Config{}, err
	}
	if err := change(&cfg); err != nil {
		return Config{}, err
	}
	cfg.Version = Version
	cfg.Revision++
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	project.Sort(cfg.Projects)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Config{}, err
	}
	f, err := os.CreateTemp(dir, ".rian-*.tmp")
	if err != nil {
		return Config{}, err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return Config{}, err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return Config{}, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return Config{}, err
	}
	if err := f.Close(); err != nil {
		return Config{}, err
	}
	if err := os.Rename(f.Name(), s.Path); err != nil {
		return Config{}, fmt.Errorf("save config: %w", err)
	}
	return cfg, nil
}

func Add(cfg *Config, p project.Project) error {
	for _, existing := range cfg.Projects {
		if project.SamePath(existing.Path, p.Path) {
			return fmt.Errorf("this directory is already saved as %s", existing.Name)
		}
	}
	cfg.Projects = append(cfg.Projects, p)
	return nil
}

func Replace(cfg *Config, p project.Project) error {
	for i := range cfg.Projects {
		if cfg.Projects[i].ID == p.ID {
			cfg.Projects[i] = p
			return nil
		}
	}
	return fmt.Errorf("project was removed by another Rian instance")
}

func Remove(cfg *Config, id string) error {
	for i, p := range cfg.Projects {
		if p.ID == id {
			cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("project no longer exists")
}

func Touch(cfg *Config, id string) error {
	for i := range cfg.Projects {
		if cfg.Projects[i].ID == id {
			cfg.Projects[i].LastUsed = time.Now()
			return nil
		}
	}
	return fmt.Errorf("project no longer exists")
}
