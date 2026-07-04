package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// integrationConfig writes a temp config with a "default" profile and points
// NOTION_CLI_CONFIG_PATH at it, so cmdXxx handlers can resolve a secret
// without touching the user's real config.
func integrationConfig(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("NOTION_CLI_CONFIG_PATH", configPath)
	cfg := Config{
		Profiles: map[string]Profile{
			"default": {NotionSecret: "secret_test"},
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	b = append(b, '\n')
	if err := os.WriteFile(configPath, b, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// mockNotion starts an httptest server that delegates to h, points the package
// at it (apiBaseURL), and restores the real base URL on cleanup.
func mockNotion(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(func() {
		ts.Close()
		apiBaseURL = "https://api.notion.com"
	})
	apiBaseURL = ts.URL
	return ts
}

// requestRecorder captures the method, path, and parsed JSON body of the last
// request seen by a mock handler.
type requestRecorder struct {
	method string
	path   string
	body   map[string]any
}

// notFoundHandler responds 404 for any path (used to assert error propagation).
func notFoundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}
}

// writeJSON writes a JSON response with a 200 status.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

const intPageID = "11111111-1111-1111-1111-111111111111"
const intDBID = "22222222-2222-2222-2222-222222222222"
const intDSID = "33333333-3333-3333-3333-333333333333"

func TestIntegration_ReadPageDefaultIsBody(t *testing.T) {
	integrationConfig(t)
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		switch r.URL.Path {
		case "/v1/pages/" + intPageID:
			writeJSON(w, map[string]any{
				"object": "page", "id": intPageID,
				"url": "https://notion.so/p/" + intPageID,
				"properties": map[string]any{
					"Title": map[string]any{"type": "title", "title": []any{map[string]any{"plain_text": "Hello"}}},
				},
			})
		case "/v1/pages/" + intPageID + "/markdown":
			writeJSON(w, map[string]any{"markdown": "# Hello\n\nWorld", "id": intPageID, "request_id": "req-1"})
		default:
			notFoundHandler()(w, r)
		}
	})

	out, err := cmdRead("default", intPageID, "", false, false, false)
	if err != nil {
		t.Fatalf("cmdRead: %v", err)
	}
	if out != "# Hello\n\nWorld" {
		t.Errorf("expected bare markdown body, got: %q", out)
	}
	if rec.path != "/v1/pages/"+intPageID+"/markdown" {
		t.Errorf("expected markdown path, got: %q", rec.path)
	}
}

func TestIntegration_ReadPageLongHasMetadata(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages/" + intPageID:
			writeJSON(w, map[string]any{"object": "page", "id": intPageID, "url": "https://notion.so/p"})
		case "/v1/pages/" + intPageID + "/markdown":
			writeJSON(w, map[string]any{"markdown": "body", "id": intPageID, "request_id": "req-1"})
		default:
			notFoundHandler()(w, r)
		}
	})
	out, err := cmdRead("default", intPageID, "", true, false, false)
	if err != nil {
		t.Fatalf("cmdRead: %v", err)
	}
	if !strings.Contains(out, "page_id: "+intPageID) {
		t.Errorf("long output missing page_id metadata, got: %q", out)
	}
}

func TestIntegration_ReadDatabaseJSON(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/databases/"+intDBID {
			writeJSON(w, map[string]any{"object": "database", "id": intDBID})
			return
		}
		notFoundHandler()(w, r)
	})
	out, err := cmdRead("default", "db:"+intDBID, "", false, false, false)
	if err != nil {
		t.Fatalf("cmdRead db: %v", err)
	}
	if !strings.Contains(out, `"object": "database"`) {
		t.Errorf("expected database JSON, got: %q", out)
	}
}

func TestIntegration_ReadSliceOnDatabaseErrors(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, notFoundHandler())
	if _, err := cmdRead("default", "db:"+intDBID, "0-2", false, false, false); err == nil {
		t.Fatal("expected error for --slice on a database, got nil")
	}
}

func TestIntegration_LsPageCompact(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/blocks/"+intPageID+"/children" {
			writeJSON(w, map[string]any{
				"object": "list",
				"results": []any{
					map[string]any{"type": "child_page", "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						"child_page": map[string]any{"title": "Child One"}},
					map[string]any{"type": "paragraph", "id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
						"paragraph": map[string]any{}},
				},
				"has_more":    false,
				"next_cursor": "",
				"request_id":  "req-ls",
			})
			return
		}
		notFoundHandler()(w, r)
	})
	out, err := cmdLs("default", intPageID, false, false, 100, "", "", "", false, "")
	if err != nil {
		t.Fatalf("cmdLs: %v", err)
	}
	if !strings.Contains(out, "child_page aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa Child One") {
		t.Errorf("expected compact child_page line, got: %q", out)
	}
	if !strings.Contains(out, "paragraph bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb") {
		t.Errorf("expected compact paragraph line, got: %q", out)
	}
}

func TestIntegration_LsDatabaseDataSources(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/databases/"+intDBID {
			writeJSON(w, map[string]any{
				"object": "database", "id": intDBID,
				"data_sources": []any{
					map[string]any{"id": intDSID, "name": "Default", "object": "data_source"},
				},
			})
			return
		}
		notFoundHandler()(w, r)
	})
	out, err := cmdLs("default", "db:"+intDBID, false, false, 100, "", "", "", false, "")
	if err != nil {
		t.Fatalf("cmdLs db: %v", err)
	}
	if !strings.Contains(out, "data_source "+intDSID+" Default") {
		t.Errorf("expected data_source line, got: %q", out)
	}
}

func TestIntegration_LsDataSourceRows(t *testing.T) {
	integrationConfig(t)
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		if r.URL.Path == "/v1/data_sources/"+intDSID+"/query" {
			writeJSON(w, map[string]any{
				"object": "list",
				"results": []any{
					map[string]any{"object": "page", "id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
						"properties": map[string]any{"Name": map[string]any{"type": "title", "title": []any{map[string]any{"plain_text": "Row A"}}}}},
				},
				"has_more": false,
			})
			return
		}
		notFoundHandler()(w, r)
	})
	out, err := cmdLs("default", "ds:"+intDSID, false, false, 10, "", "", "", false, "")
	if err != nil {
		t.Fatalf("cmdLs ds: %v", err)
	}
	if !strings.Contains(out, "page cccccccc-cccc-cccc-cccc-cccccccccccc Row A") {
		t.Errorf("expected page row line, got: %q", out)
	}
	if rec.method != http.MethodPost {
		t.Errorf("expected POST to query, got %s", rec.method)
	}
}

func TestIntegration_LsFilterOnPageErrors(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, notFoundHandler())
	if _, err := cmdLs("default", intPageID, false, false, 100, "", `{"x":1}`, "", false, ""); err == nil {
		t.Fatal("expected error for --filter on a page, got nil")
	}
}

func TestIntegration_FindCompact(t *testing.T) {
	integrationConfig(t)
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		writeJSON(w, map[string]any{
			"object": "list",
			"results": []any{
				map[string]any{"object": "page", "id": intPageID,
					"properties": map[string]any{"Title": map[string]any{"type": "title", "title": []any{map[string]any{"plain_text": "Found"}}}}},
			},
			"has_more": false,
		})
	})
	out, err := cmdFind("default", "term", "last_edited_time", "descending", "", 10, false, false)
	if err != nil {
		t.Fatalf("cmdFind: %v", err)
	}
	if !strings.Contains(out, "page "+intPageID+" Found") {
		t.Errorf("expected compact find line, got: %q", out)
	}
	if rec.path != "/v1/search" || rec.method != http.MethodPost {
		t.Errorf("expected POST /v1/search, got %s %s", rec.method, rec.path)
	}
	if asString(rec.body["query"]) != "term" {
		t.Errorf("expected query=term in body, got %v", rec.body["query"])
	}
}

func TestIntegration_FindBadSortDirection(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, notFoundHandler())
	if _, err := cmdFind("default", "x", "last_edited_time", "sideways", "", 10, false, false); err == nil {
		t.Fatal("expected error for bad sort-direction, got nil")
	}
}

func TestIntegration_MkdbPostsDatabase(t *testing.T) {
	integrationConfig(t)
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		if r.URL.Path == "/v1/databases" {
			writeJSON(w, map[string]any{
				"object": "database", "id": intDBID,
				"data_sources": []any{map[string]any{"id": intDSID}},
			})
			return
		}
		notFoundHandler()(w, r)
	})
	out, err := cmdMkdb("default", "Tracker", intPageID, "")
	if err != nil {
		t.Fatalf("cmdMkdb: %v", err)
	}
	if rec.method != http.MethodPost || rec.path != "/v1/databases" {
		t.Errorf("expected POST /v1/databases, got %s %s", rec.method, rec.path)
	}
	parent := asMap(rec.body["parent"])
	if asString(parent["type"]) != "page_id" || asString(parent["page_id"]) != intPageID {
		t.Errorf("expected parent page_id=%s, got %v", intPageID, parent)
	}
	if !strings.Contains(out, intDSID) {
		t.Errorf("expected output to contain data source id, got: %q", out)
	}
}

func TestIntegration_MkdbParentMustBePage(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, notFoundHandler())
	if _, err := cmdMkdb("default", "Tracker", "db:"+intDBID, ""); err == nil {
		t.Fatal("expected error for mkdb with db parent, got nil")
	}
}

func TestIntegration_WriteCreatesPage(t *testing.T) {
	integrationConfig(t)
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		if r.URL.Path == "/v1/pages" {
			writeJSON(w, map[string]any{
				"object": "page", "id": intPageID, "url": "https://notion.so/p",
				"parent": map[string]any{"type": "page_id", "page_id": "pppppppp-pppp-pppp-pppp-pppppppppppp"},
			})
			return
		}
		notFoundHandler()(w, r)
	})
	out, err := cmdWrite("default", "Notes", intPageID, "# body", "")
	if err != nil {
		t.Fatalf("cmdWrite: %v", err)
	}
	if rec.method != http.MethodPost || rec.path != "/v1/pages" {
		t.Errorf("expected POST /v1/pages, got %s %s", rec.method, rec.path)
	}
	if asString(rec.body["markdown"]) != "# body" {
		t.Errorf("expected markdown body, got %v", rec.body["markdown"])
	}
	if !strings.Contains(out, "# ✅ Created Page") {
		t.Errorf("expected created-page banner, got: %q", out)
	}
}

func TestIntegration_EditReplace(t *testing.T) {
	integrationConfig(t)
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		switch r.URL.Path {
		case "/v1/pages/" + intPageID + "/markdown":
			rec.method, rec.path = r.Method, r.URL.Path
			writeJSON(w, map[string]any{"id": intPageID, "request_id": "req-e"})
		case "/v1/pages/" + intPageID:
			writeJSON(w, map[string]any{"object": "page", "id": intPageID, "url": "https://notion.so/p"})
		default:
			notFoundHandler()(w, r)
		}
	})
	out, err := cmdEdit("default", intPageID, true, "new body", "", nil, nil, false, false)
	if err != nil {
		t.Fatalf("cmdEdit: %v", err)
	}
	if rec.method != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", rec.method)
	}
	if asString(rec.body["type"]) != "replace_content" {
		t.Errorf("expected replace_content mode, got %v", rec.body["type"])
	}
	if !strings.Contains(out, "mode: replace_content") {
		t.Errorf("expected replace_content in output, got: %q", out)
	}
}

func TestIntegration_MvPostsParent(t *testing.T) {
	integrationConfig(t)
	const destParent = "44444444-4444-4444-4444-444444444444"
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		if r.URL.Path == "/v1/pages/"+intPageID {
			rec.method, rec.path = r.Method, r.URL.Path
			writeJSON(w, map[string]any{"object": "page", "id": intPageID, "url": "https://notion.so/p",
				"parent": map[string]any{"type": "page_id", "page_id": destParent}})
			return
		}
		notFoundHandler()(w, r)
	})
	_, err := cmdMv("default", intPageID, destParent)
	if err != nil {
		t.Fatalf("cmdMv: %v", err)
	}
	if rec.method != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", rec.method)
	}
	parent := asMap(rec.body["parent"])
	if asString(parent["page_id"]) != destParent {
		t.Errorf("expected parent page_id=%s, got %v", destParent, parent)
	}
}

func TestIntegration_MvSelfMoveErrors(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, notFoundHandler())
	if _, err := cmdMv("default", intPageID, intPageID); err == nil {
		t.Fatal("expected error for self-move, got nil")
	}
}

func TestIntegration_RmPagePatchesInTrash(t *testing.T) {
	integrationConfig(t)
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		if r.URL.Path == "/v1/pages/"+intPageID {
			writeJSON(w, map[string]any{"id": intPageID, "in_trash": true, "request_id": "req-r"})
			return
		}
		notFoundHandler()(w, r)
	})
	out, err := cmdRm("default", intPageID)
	if err != nil {
		t.Fatalf("cmdRm: %v", err)
	}
	if rec.method != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", rec.method)
	}
	if v, _ := rec.body["in_trash"].(bool); !v {
		t.Errorf("expected in_trash=true in body, got %v", rec.body["in_trash"])
	}
	if !strings.Contains(out, "in_trash: true") {
		t.Errorf("expected in_trash: true in output, got: %q", out)
	}
}

func TestIntegration_RmDatabaseTargetsDatabaseEndpoint(t *testing.T) {
	integrationConfig(t)
	var rec requestRecorder
	mockNotion(t, func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		if r.URL.Path == "/v1/databases/"+intDBID {
			writeJSON(w, map[string]any{"id": intDBID, "in_trash": true})
			return
		}
		notFoundHandler()(w, r)
	})
	if _, err := cmdRm("default", "db:"+intDBID); err != nil {
		t.Fatalf("cmdRm db: %v", err)
	}
	if rec.path != "/v1/databases/"+intDBID {
		t.Errorf("expected database endpoint, got %q", rec.path)
	}
}

func TestIntegration_Read404Propagates(t *testing.T) {
	integrationConfig(t)
	mockNotion(t, notFoundHandler())
	if _, err := cmdRead("default", intPageID, "", false, false, false); err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}
