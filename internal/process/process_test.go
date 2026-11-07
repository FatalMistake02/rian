package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func helperCommand(t *testing.T, mode, dir string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(), "RIAN_TEST_HELPER="+mode, "RIAN_TEST_HELPER_DIR="+dir)
	cmd.Dir = dir
	return cmd
}

func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("RIAN_TEST_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "output":
		fmt.Fprintln(os.Stdout, "hello from stdout")
		fmt.Fprintln(os.Stderr, "hello from stderr")
	case "long":
		fmt.Fprintln(os.Stdout, "ready")
		time.Sleep(time.Minute)
	case "tree":
		exe, _ := os.Executable()
		child := exec.Command(exe, "-test.run=^TestProcessHelper$")
		child.Env = append(os.Environ(), "RIAN_TEST_HELPER=child")
		if err := child.Start(); err != nil {
			fmt.Fprintln(os.Stdout, err)
			os.Exit(2)
		}
		os.WriteFile(filepath.Join(os.Getenv("RIAN_TEST_HELPER_DIR"), "child.pid"), []byte(fmt.Sprint(child.Process.Pid)), 0600)
		fmt.Fprintln(os.Stdout, "ready")
		time.Sleep(time.Minute)
	case "child":
		file, err := os.OpenFile(filepath.Join(os.Getenv("RIAN_TEST_HELPER_DIR"), "child.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			os.Exit(3)
		}
		defer file.Close()
		for range 1000 {
			file.WriteString(".")
			file.Sync()
			time.Sleep(20 * time.Millisecond)
		}
	}
	os.Exit(0)
}

func until(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("process state did not arrive")
}

func TestCaptureAndOneProcessPerProject(t *testing.T) {
	manager := New()
	defer manager.Close()
	dir := t.TempDir()
	if _, err := manager.StartCommand("p", "helper output", helperCommand(t, "output", dir)); err != nil {
		t.Fatal(err)
	}
	until(t, func() bool { return !manager.Snapshot("p").Running })
	s := manager.Snapshot("p")
	if !strings.Contains(s.Output, "stdout") || !strings.Contains(s.Output, "stderr") || s.PID == 0 || s.Started.IsZero() || s.Ended.IsZero() {
		t.Fatalf("missing process information: %+v", s)
	}
	if _, err := manager.StartCommand("p", "long", helperCommand(t, "long", dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartCommand("p", "duplicate", helperCommand(t, "long", dir)); err == nil {
		t.Fatal("a second process for one project was accepted")
	}
	if err := manager.Stop("p"); err != nil {
		t.Fatal(err)
	}
	if s = manager.Snapshot("p"); s.Running || !s.Stopped || s.Error != "" {
		t.Fatalf("stop failed: %+v", s)
	}
}

func TestStopIncludesChildProcess(t *testing.T) {
	manager := New()
	defer manager.Close()
	dir := t.TempDir()
	if _, err := manager.StartCommand("tree", "tree", helperCommand(t, "tree", dir)); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "child.log")
	until(t, func() bool { info, err := os.Stat(file); return err == nil && info.Size() > 0 })
	if err := manager.Stop("tree"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(file)
	time.Sleep(200 * time.Millisecond)
	after, _ := os.Stat(file)
	if before.Size() != after.Size() {
		t.Fatal("a descendant process kept running after stop")
	}
}

func TestOutputIsBounded(t *testing.T) {
	s := &session{}
	s.Write([]byte(strings.Repeat("a", outputLimit*2)))
	s.Write([]byte("tail"))
	state := s.snapshot()
	if len(state.Output) != outputLimit || !strings.HasSuffix(state.Output, "tail") {
		t.Fatal("recent output was not bounded correctly")
	}
}

func TestShutdownRejectsLateStarts(t *testing.T) {
	manager := New()
	manager.Close()
	if _, err := manager.StartCommand("late", "late", helperCommand(t, "long", t.TempDir())); err == nil {
		t.Fatal("a process started after shutdown")
	}
}
