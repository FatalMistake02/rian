package tools

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestTerminalArgumentsPreservePaths(t *testing.T) {
	child := exec.Command("/tmp/tools/cli with spaces", "literal $value", "quote'and\"quote")
	child.Dir = "/tmp/project folder"
	for _, mode := range []string{"window"} {
		cmd, err := terminalCommand("konsole", child, mode)
		if err != nil {
			t.Fatal(err)
		}
		flag := "--separate"
		want := append([]string{"konsole", flag, "--hold", "--workdir", child.Dir, "-e"}, child.Args...)
		if !reflect.DeepEqual(cmd.Args, want) || cmd.Dir != child.Dir {
			t.Fatalf("got %q in %q", cmd.Args, cmd.Dir)
		}
	}
	if _, err := terminalCommand("konsole", child, "tab"); err == nil {
		t.Fatal("unsupported tab accepted")
	}
	if _, err := terminalCommand("", child, "window"); err == nil {
		t.Fatal("missing terminal accepted")
	}
}
