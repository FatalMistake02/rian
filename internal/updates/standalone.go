package updates

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func assetName(version, platform, arch string) (string, error) {
	if _, ok := parts(version); !ok {
		return "", fmt.Errorf("invalid release version")
	}
	if platform != "darwin" && platform != "linux" && platform != "windows" || arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("unsupported platform %s/%s", platform, arch)
	}
	ext := ".tar.gz"
	if platform == "windows" {
		ext = ".zip"
	}
	return "rian_" + strings.TrimPrefix(version, "v") + "_" + platform + "_" + arch + ext, nil
}

func download(client *http.Client, url string, max int64) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("download exceeds size limit")
	}
	return data, nil
}

func verifiedArchive(client *http.Client, base, name string) ([]byte, error) {
	sums, err := download(client, base+"/SHA256SUMS", 1<<20)
	if err != nil {
		return nil, err
	}
	var checksum string
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			if checksum != "" {
				return nil, fmt.Errorf("duplicate checksum for %s", name)
			}
			checksum = fields[0]
		}
	}
	want, err := hex.DecodeString(checksum)
	if err != nil || len(want) != sha256.Size {
		return nil, fmt.Errorf("missing or invalid checksum for %s", name)
	}
	data, err := download(client, base+"/"+name, 64<<20)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(data)
	if !bytes.Equal(got[:], want) {
		return nil, fmt.Errorf("checksum mismatch for %s", name)
	}
	return data, nil
}

// Extract only the three known regular files; archive paths are never used on disk.
func extract(data []byte, windows bool) (map[string][]byte, error) {
	binary := "rian"
	if windows {
		binary = "rian.exe"
	}
	files := map[string][]byte{}
	read := func(name string, regular bool, r io.Reader) error {
		if name != binary && name != "LICENSE" && name != "THIRD_PARTY_NOTICES.txt" {
			return fmt.Errorf("unexpected archive entry %q", name)
		}
		if !regular || files[name] != nil {
			return fmt.Errorf("invalid archive entry %q", name)
		}
		b, err := io.ReadAll(io.LimitReader(r, 64<<20+1))
		if err != nil {
			return err
		}
		if len(b) == 0 || len(b) > 64<<20 {
			return fmt.Errorf("invalid archive entry size")
		}
		files[name] = b
		return nil
	}
	if windows {
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range z.File {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = read(f.Name, f.Mode().IsRegular(), r)
			_ = r.Close()
			if err != nil {
				return nil, err
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if err := read(h.Name, h.Typeflag == tar.TypeReg, tr); err != nil {
				return nil, err
			}
		}
	}
	if len(files) != 3 {
		return nil, fmt.Errorf("release archive is incomplete")
	}
	return files, nil
}

func UpdateStandalone(version string) error {
	name, err := assetName(version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	i, err := Detect()
	if err != nil {
		return err
	}
	if i.Method != "standalone" {
		return fmt.Errorf("not a standalone installation")
	}
	base := "https://github.com/" + Repository + "/releases/download/v" + strings.TrimPrefix(version, "v")
	client := &http.Client{Timeout: 2 * time.Minute}
	data, err := verifiedArchive(client, base, name)
	if err != nil {
		return err
	}
	files, err := extract(data, runtime.GOOS == "windows")
	if err != nil {
		return err
	}
	return replaceStandalone(i.Executable, files)
}

func replaceStandalone(exe string, files map[string][]byte) error {
	dir := filepath.Dir(exe)
	stage, err := os.MkdirTemp(dir, ".rian-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for name, data := range files {
		if name != filepath.Base(exe) && name != "LICENSE" && name != "THIRD_PARTY_NOTICES.txt" {
			return fmt.Errorf("unexpected file %s", name)
		}
		mode := os.FileMode(0644)
		if name == filepath.Base(exe) {
			mode = 0755
		}
		if err := os.WriteFile(filepath.Join(stage, name), data, mode); err != nil {
			return err
		}
	}
	// Place notices before replacing the executable. Failure leaves the old executable usable.
	for _, name := range []string{"LICENSE", "THIRD_PARTY_NOTICES.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), files[name], 0644); err != nil {
			return err
		}
	}
	newExe := filepath.Join(stage, filepath.Base(exe))
	if runtime.GOOS != "windows" {
		return os.Rename(newExe, exe)
	}
	// Windows can rename an executing image but cannot overwrite it in place.
	backup := exe + ".old"
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(exe, backup); err != nil {
		return err
	}
	if err := os.Rename(newExe, exe); err != nil {
		if rollback := os.Rename(backup, exe); rollback != nil {
			return fmt.Errorf("update failed: %v; restore %s manually: %w", err, backup, rollback)
		}
		return err
	}
	_ = os.Remove(backup) // The old running image may remain locked until restart.
	return nil
}
