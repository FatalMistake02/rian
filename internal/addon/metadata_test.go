package addon

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rian/internal/project"
)

func metadataFixture(t *testing.T, endpoint, contents string) (Installed, project.Project) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	item := Installed{Enabled: true, Definition: Definition{FormatVersion: 1, Version: "1.0.0", ID: "registry", Name: "Registry", Detect: Detection{Files: []string{"manifest.json"}},
		Metadata: &MetadataProvider{File: "manifest.json", Fetch: endpoint + "/${file.name}", FetchWhen: []MetadataCondition{{Path: "file.private", Op: "ne", Value: "true"}}, Name: "${file.name}", Summary: "local ${file.version} / remote ${remote.version}", URL: "https://example.com/items/${file.name}", URLWhen: []MetadataCondition{{Path: "request.status", Op: "eq", Value: "200"}, {Path: "request.error", Op: "empty"}}, Status: "Available", Statuses: []MetadataStatus{{When: []MetadataCondition{{Path: "file.private", Op: "eq", Value: "true"}}, Text: "Private"}, {When: []MetadataCondition{{Path: "request.error", Op: "present"}}, Text: "Failed: ${request.error}"}, {When: []MetadataCondition{{Path: "request.status", Op: "eq", Value: "404"}}, Text: "Not published"}}, Fields: []MetadataField{{Label: "Version", Value: "${remote.version}"}}},
		Actions:  []Action{{ID: "open", Name: "Open", Kind: "metadata-url"}}}}
	return item, project.New("Example", root)
}

func TestMetadataFetchAndConditions(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.EscapedPath() != "/@group%2Fwidget" {
			t.Errorf("unescaped package path: %s", r.URL.EscapedPath())
		}
		fmt.Fprint(w, `{"version":"2.0.0"}`)
	}))
	defer server.Close()
	item, p := metadataFixture(t, server.URL, `{"name":"@group/widget","version":"1.0.0"}`)
	if err := item.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := item.LoadMetadata(context.Background(), p)
	if err != nil || data.Name != "@group/widget" || !strings.Contains(data.Summary, "2.0.0") || data.URL == "" || len(data.Fields) != 1 {
		t.Fatalf("metadata: %+v %v", data, err)
	}
	if err := os.WriteFile(filepath.Join(p.Path, "manifest.json"), []byte(`{"name":"secret","private":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	data, err = item.LoadMetadata(context.Background(), p)
	if err != nil || requests != 1 || data.Status != "Private" || data.URL != "" {
		t.Fatalf("private data leaked: %+v requests=%d err=%v", data, requests, err)
	}
}

func TestMetadataFailuresAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       int
		body, want string
	}{
		{"missing", 404, `{}`, "Not published"}, {"unavailable", 503, `{}`, "Failed: HTTP 503"},
		{"malformed", 200, `no json`, "Failed: Invalid JSON response"}, {"oversized", 200, strings.Repeat(" ", 1<<20) + "{}", "Failed: Invalid JSON response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			item, p := metadataFixture(t, server.URL, `{"name":"widget"}`)
			data, err := item.LoadMetadata(context.Background(), p)
			if err != nil || data.Status != tc.want || data.URL != "" {
				t.Fatalf("unexpected result: %+v %v", data, err)
			}
		})
	}
	item, p := metadataFixture(t, "https://example.com", `{"name":"widget"} {}`)
	if _, err := item.LoadMetadata(context.Background(), p); err == nil {
		t.Fatal("accepted trailing local JSON")
	}
	item, p = metadataFixture(t, "https://example.com", `{"name":"widget"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, err := item.LoadMetadata(ctx, p)
	if err != nil || !strings.Contains(data.Status, "Request failed") {
		t.Fatalf("cancellation: %+v %v", data, err)
	}
}

func TestMetadataValidation(t *testing.T) {
	values := metadataValues{"file": map[string]any{"name": "@group/widget"}}
	if got := values.expand("https://example.com/${file.name|path}", true); got != "https://example.com/@group/widget" {
		t.Fatalf("path encoding: %s", got)
	}
	for _, change := range []func(*MetadataProvider){
		func(p *MetadataProvider) { p.File = "../secret.json" }, func(p *MetadataProvider) { p.File = "C:/secret.json" },
		func(p *MetadataProvider) { p.Fetch = "file:///secret" }, func(p *MetadataProvider) { p.Fetch = "https://${file.host}/data" },
		func(p *MetadataProvider) { p.Fetch = "https://user:pass@example.com/data" }, func(p *MetadataProvider) { p.Name = "${unknown.field}" },
		func(p *MetadataProvider) { p.FetchWhen = []MetadataCondition{{Path: "file.name", Op: "execute"}} },
	} {
		p := MetadataProvider{File: "manifest.json"}
		change(&p)
		if err := p.Validate(nil); err == nil {
			t.Fatalf("accepted invalid metadata: %+v", p)
		}
	}
}

func TestMetadataRemoteFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"items":[{"title":"Latest post","url":"hello-world"}]}`)
	}))
	defer server.Close()
	item, p := metadataFixture(t, server.URL, `{}`)
	item.Definition.Metadata = &MetadataProvider{Fetch: server.URL, Name: "${remote.items.0.title}", URL: "https://example.com/posts/${remote.items.0.url}"}
	if err := item.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := item.LoadMetadata(context.Background(), p)
	if err != nil || data.Name != "Latest post" || data.URL != "https://example.com/posts/hello-world" {
		t.Fatalf("feed: %+v %v", data, err)
	}
}

func TestMetadataVisibilitySetting(t *testing.T) {
	item, p := metadataFixture(t, "https://example.com", `{"name":"private-app","private":true}`)
	item.Definition.Settings = []Setting{{ID: "published-only", Name: "Published only", Type: "bool", Default: true}}
	item.Definition.Metadata.FilterSetting = "published-only"
	item.Definition.Metadata.VisibleWhen = []MetadataCondition{{Path: "request.status", Op: "eq", Value: "200"}}
	if err := item.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := item.LoadMetadata(context.Background(), p)
	if err != nil || !data.Hidden {
		t.Fatalf("default filter failed: %+v %v", data, err)
	}
	item.Values = map[string]string{"published-only": "false"}
	data, err = item.LoadMetadata(context.Background(), p)
	if err != nil || data.Hidden || data.Status != "Private" {
		t.Fatalf("disabled filter failed: %+v %v", data, err)
	}
	item.Definition.Metadata.FilterSetting = "missing"
	if err := item.Validate(); err == nil {
		t.Fatal("accepted nonexistent filter setting")
	}
}
