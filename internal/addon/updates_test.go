package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func updateFixture(id, version, url string) Installed {
	return Installed{Enabled: true, Source: "original.json", Definition: Definition{
		FormatVersion: 1, Version: version, ID: id, Name: "Example", UpdateURL: url,
		Actions: []Action{{ID: "open", Name: "Open", Kind: "url", URL: "https://example.com"}},
	}, Values: map[string]string{}}
}

func TestCheckUpdatesValidatesEachManifestIndependently(t *testing.T) {
	valid := updateFixture("newer", "1.1.0", "").Definition
	encode := func(value Definition) string {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	bodies := map[string]string{"newer": encode(valid), "wrong-id": encode(valid), "bad-json": "{", "too-large": strings.Repeat(" ", 1024*1024+1)}
	for _, tc := range []struct{ id, version string }{{"same", "1.0.0"}, {"older", "0.9.0"}, {"unsupported", "1.1.0"}, {"trailing", "1.1.0"}} {
		d := updateFixture(tc.id, tc.version, "").Definition
		if tc.id == "unsupported" {
			d.FormatVersion = 2
		}
		bodies[tc.id] = encode(d)
		if tc.id == "trailing" {
			bodies[tc.id] += " {}"
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected update request: %s, Accept=%s", r.Method, r.Header.Get("Accept"))
		}
		id := strings.TrimPrefix(r.URL.Path, "/")
		if id == "offline" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, bodies[id])
	}))
	defer server.Close()
	installed := map[string]Installed{"no-url": updateFixture("no-url", "1.0.0", "")}
	for id := range bodies {
		installed[id] = updateFixture(id, "1.0.0", server.URL+"/"+id)
	}
	installed["offline"] = updateFixture("offline", "1.0.0", server.URL+"/offline")
	installed["newer"] = updateFixture("newer", "1.0.0", server.URL+"/newer")
	item := installed["newer"]
	item.Enabled = false
	installed["newer"] = item
	before, err := json.Marshal(installed)
	if err != nil {
		t.Fatal(err)
	}
	results := CheckUpdates(context.Background(), installed)
	if _, checked := results["no-url"]; checked {
		t.Fatal("addon without update URL was checked")
	}
	if got := results["newer"]; got.Err != nil || got.Definition.Version != "1.1.0" {
		t.Fatalf("newer release missing: %+v", got)
	}
	for _, id := range []string{"same", "older"} {
		if got := results[id]; got.Err != nil || got.Definition.Version != "" {
			t.Errorf("%s was offered as an update: %+v", id, got)
		}
	}
	for _, id := range []string{"wrong-id", "bad-json", "too-large", "unsupported", "trailing", "offline"} {
		if got := results[id]; got.Err == nil || got.Definition.Version != "" {
			t.Errorf("invalid update %s accepted: %+v", id, got)
		}
	}
	after, _ := json.Marshal(installed)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("checking for updates modified installed addons")
	}
}

func TestCheckUpdatesCancellationAndConcurrencyLimit(t *testing.T) {
	var active, maximum atomic.Int32
	started := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	installed := map[string]Installed{}
	for i := range 6 {
		id := fmt.Sprintf("example-%d", i)
		installed[id] = updateFixture(id, "1.0.0", server.URL)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan map[string]UpdateResult, 1)
	go func() { done <- CheckUpdates(ctx, installed) }()
	for range 4 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("update requests did not run concurrently")
		}
	}
	cancel()
	select {
	case results := <-done:
		if len(results) != len(installed) {
			t.Fatalf("canceled checks lost results: %d", len(results))
		}
		for id, result := range results {
			if result.Err == nil {
				t.Errorf("canceled check %s succeeded", id)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("update checks ignored cancellation")
	}
	if got := maximum.Load(); got != 4 {
		t.Fatalf("concurrency limit: got %d, want 4", got)
	}
}
