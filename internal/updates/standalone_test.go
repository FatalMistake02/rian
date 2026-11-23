package updates

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func archive(t *testing.T, windows bool, extra string) []byte {
	t.Helper()
	var b bytes.Buffer
	files := map[string]string{"LICENSE": "license", "THIRD_PARTY_NOTICES.txt": "notices"}
	if windows {
		files["rian.exe"] = "binary"
	} else {
		files["rian"] = "binary"
	}
	if extra != "" {
		files[extra] = "bad"
	}
	if windows {
		z := zip.NewWriter(&b)
		for name, data := range files {
			w, err := z.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&b)
		tw := tar.NewWriter(gz)
		for name, data := range files {
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return b.Bytes()
}

func TestArchivesAndUnsafeEntries(t *testing.T) {
	for _, windows := range []bool{false, true} {
		files, err := extract(archive(t, windows, ""), windows)
		if err != nil || len(files) != 3 {
			t.Fatalf("extract: %v", err)
		}
		for _, name := range []string{"../rian", "/tmp/rian", "unexpected"} {
			if _, err := extract(archive(t, windows, name), windows); err == nil {
				t.Fatalf("accepted %q", name)
			}
		}
	}
}

func TestDownloadRequiresMatchingChecksum(t *testing.T) {
	data := archive(t, false, "")
	name := "rian_1.0.0_linux_amd64.tar.gz"
	checksum := fmt.Sprintf("%x", sha256.Sum256(data))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHA256SUMS") {
			fmt.Fprintf(w, "%s  %s\n", checksum, name)
		} else {
			_, _ = w.Write(data)
		}
	}))
	defer srv.Close()
	if _, err := verifiedArchive(srv.Client(), srv.URL, name); err != nil {
		t.Fatal(err)
	}
	checksum = strings.Repeat("0", 64)
	if _, err := verifiedArchive(srv.Client(), srv.URL, name); err == nil {
		t.Fatal("accepted corrupt archive")
	}
}

func TestReleaseChecksBothRegistries(t *testing.T) {
	for _, body := range []string{`{"version":"1.2.3"}`, `{"tag_name":"v1.2.3"}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		v, err := check("1.0.0", srv.Client(), srv.URL)
		srv.Close()
		if err != nil || v != "1.2.3" {
			t.Fatalf("check: %q %v", v, err)
		}
	}
}

func TestReplaceRunningExecutable(t *testing.T) {
	if os.Getenv("RIAN_TEST_REPLACE") == "1" {
		exe, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		files := map[string][]byte{filepath.Base(exe): []byte("replacement"), "LICENSE": []byte("license"), "THIRD_PARTY_NOTICES.txt": []byte("notices")}
		if err := replaceStandalone(exe, files); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		os.Exit(0)
	}
	dir := t.TempDir()
	name := "rian"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	copy := filepath.Join(dir, name)
	if err := os.WriteFile(copy, data, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(copy, "-test.run=^TestReplaceRunningExecutable$")
	cmd.Env = append(os.Environ(), "RIAN_TEST_REPLACE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("replace running binary: %s %v", out, err)
	}
	got, err := os.ReadFile(copy)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("replacement: %q %v", got, err)
	}
}
