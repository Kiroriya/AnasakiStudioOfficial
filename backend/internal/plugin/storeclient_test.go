package plugin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// fakeManager has one installed package ("countdown" 1.0.0) and no database.
func fakeManager() *Manager {
	m := &Manager{host: &Host{}, byID: map[string]*entry{}}
	m.pkgs = &packages{m: m, files: map[string]string{},
		list: []*pkg{{mf: &Manifest{ID: "countdown", Name: "Countdown"}, version: "1.0.0", active: true}}}
	return m
}

func TestStoreClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/plugins":
			if r.URL.Query().Get("category") != "marketing" {
				t.Errorf("query not forwarded: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"countdown","version":"1.2.0","iconUrl":"/api/v1/plugins/countdown/icon"},
				{"id":"other","version":"0.1.0"}],"total":2,"page":1}`))
		case "/api/v1/plugins/countdown":
			_, _ = w.Write([]byte(`{"id":"countdown","version":"1.0.0","screenshots":["/s/1.png","https://cdn.x/2.png"]}`))
		case "/api/v1/rankings":
			_, _ = w.Write([]byte(`{"topDownloads":[{"id":"countdown","version":"2.0.0"}],"topRated":[],"trending":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer srv.Close()
	t.Setenv("STORE_URL", srv.URL+"/")
	m := fakeManager()
	if m.StoreURL() != srv.URL {
		t.Fatalf("StoreURL: %s", m.StoreURL())
	}

	v, err := m.StoreJSON("GET", "/api/v1/plugins", url.Values{"category": {"marketing"}}, nil, StoreList, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	var list struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	_ = json.Unmarshal(b, &list)
	if list.Total != 2 || len(list.Items) != 2 {
		t.Fatalf("list: %s", b)
	}
	cd, other := list.Items[0], list.Items[1]
	if cd["installed"] != true || cd["installedVersion"] != "1.0.0" || cd["updateAvailable"] != true ||
		cd["iconUrl"] != srv.URL+"/api/v1/plugins/countdown/icon" {
		t.Errorf("countdown: %v", cd)
	}
	if other["installed"] != false || other["installedVersion"] != "" || other["updateAvailable"] != false {
		t.Errorf("other: %v", other)
	}

	v, err = m.StoreJSON("GET", "/api/v1/plugins/countdown", nil, nil, StoreDetail, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := v.(map[string]any)
	if d["updateAvailable"] != false || d["installed"] != true || d["screenshots"].([]any)[0] != srv.URL+"/s/1.png" {
		t.Errorf("detail: %v", d)
	}
	v, _ = m.StoreJSON("GET", "/api/v1/rankings", nil, nil, StoreRankings, nil)
	if top := v.(map[string]any)["topDownloads"].([]any)[0].(map[string]any); top["updateAvailable"] != true {
		t.Errorf("rankings: %v", top)
	}

	_, err = m.StoreJSON("GET", "/api/v1/plugins/nope", nil, nil, StoreDetail, nil)
	var se *StoreError
	if !errors.As(err, &se) || se.Status != 404 {
		t.Errorf("want StoreError 404, got %v", err)
	}

	t.Setenv("STORE_URL", "http://127.0.0.1:1") // nothing listens there
	if _, err := m.StoreJSON("GET", "/api/v1/categories", nil, nil, StoreRaw, nil); !errors.Is(err, ErrStoreUnavailable) {
		t.Errorf("want ErrStoreUnavailable, got %v", err)
	}
	if _, err := m.InstallFromStore("../etc", ""); err == nil {
		t.Error("invalid id accepted")
	}
}
