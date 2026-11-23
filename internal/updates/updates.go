// Package updates checks releases and updates the installation that is running.
package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	return check(current, http.DefaultClient, releaseEndpoint())
}

func check(current string, client *http.Client, endpoint string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release check: %s", resp.Status)
	}
	var release struct {
		Version string `json:"version"`
		Tag     string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return "", err
	}
	if release.Version == "" {
		release.Version = strings.TrimPrefix(release.Tag, "v")
	}
	if _, valid := parts(release.Version); !valid {
		return "", fmt.Errorf("invalid release version")
	}
	if Newer(release.Version, current) {
		return release.Version, nil
	}
	return "", nil
}
