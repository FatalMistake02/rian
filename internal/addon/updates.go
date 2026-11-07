package addon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const updateTimeout = 8 * time.Second

// UpdateResult contains a newer manifest, no manifest when already current,
// or an error for this addon. A failed check never changes an installed addon.
type UpdateResult struct {
	Definition Definition
	Err        error
}

// CheckUpdates checks configured update URLs without running addon commands.
// Calls are bounded as a group and limited to four simultaneous requests.
func CheckUpdates(ctx context.Context, installed map[string]Installed) map[string]UpdateResult {
	type job struct {
		id   string
		item Installed
	}
	jobs := make(chan job, len(installed))
	for id, item := range installed {
		if item.Definition.UpdateURL != "" {
			jobs <- job{id, item}
		}
	}
	count := len(jobs)
	close(jobs)
	results := make(map[string]UpdateResult, count)
	if count == 0 {
		return results
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	client := &http.Client{Timeout: updateTimeout}
	var mu sync.Mutex
	var workers sync.WaitGroup
	for range min(4, count) {
		workers.Go(func() {
			for task := range jobs {
				result := checkUpdate(ctx, client, task.id, task.item)
				mu.Lock()
				results[task.id] = result
				mu.Unlock()
			}
		})
	}
	workers.Wait()
	return results
}

func checkUpdate(ctx context.Context, client *http.Client, id string, current Installed) UpdateResult {
	if current.Definition.ID != id {
		return UpdateResult{Err: fmt.Errorf("addon id mismatch")}
	}
	if err := current.Definition.Validate(); err != nil {
		return UpdateResult{Err: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, current.Definition.UpdateURL, nil)
	if err != nil {
		return UpdateResult{Err: err}
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return UpdateResult{Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return UpdateResult{Err: fmt.Errorf("addon update: %s", response.Status)}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxManifestSize+1))
	if err != nil {
		return UpdateResult{Err: err}
	}
	next, err := Parse(data)
	if err != nil {
		return UpdateResult{Err: err}
	}
	if next.ID != id {
		return UpdateResult{Err: fmt.Errorf("addon update has id %q; expected %q", next.ID, id)}
	}
	if !Newer(next.Version, current.Definition.Version) {
		return UpdateResult{}
	}
	return UpdateResult{Definition: next}
}
