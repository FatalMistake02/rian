package tools

import (
	"os/exec"
	"runtime"
	"testing"
)

func TestFolderPickerResults(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux picker exit conventions")
	}
	for _, tc := range []struct {
		name, script, want string
		wantErr            bool
	}{
		{"cancel with warning", "printf 'GTK warning\\n' >&2; exit 1", "", false},
		{"cancel", "exit 1", "", false},
		{"selection", "printf '/tmp/project folder\\n'", "/tmp/project folder", false},
		{"failure", "printf 'display unavailable\\n' >&2; exit 255", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, err := runFolderPicker(exec.Command("sh", "-c", tc.script))
			if path != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("path=%q err=%v", path, err)
			}
		})
	}
}
