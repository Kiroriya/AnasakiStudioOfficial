package plugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// OfficialStoreURL is the Anasaki Plugin Store every installation talks to.
// It is compiled in on purpose: site owners cannot point their CMS at another
// store from the admin. Developers can override it with the STORE_URL
// environment variable (e.g. a local store on http://localhost:8095).
const OfficialStoreURL = "https://storecms.anasaki.live"

// ErrStoreUnavailable means the plugin store could not be reached.
var ErrStoreUnavailable = errors.New("plugin store unavailable")

// StoreError is an error answer of the store (status ≥ 400), passed on.
type StoreError struct {
	Status int
	Body   []byte
}

func (e *StoreError) Error() string { return fmt.Sprintf("plugin store: status %d", e.Status) }

var storeClient = &http.Client{Timeout: 15 * time.Second}

// StoreURL returns the plugin store base URL without a trailing slash:
// STORE_URL when set (development), otherwise the official store.
func (m *Manager) StoreURL() string {
	u := strings.TrimSpace(os.Getenv("STORE_URL"))
	if u == "" {
		u = OfficialStoreURL
	}
	return strings.TrimRight(u, "/")
}

// storeDo calls the store and returns the raw body of a 2xx answer.
func (m *Manager) storeDo(method, p string, q url.Values, body []byte, hdr http.Header, limit int64) ([]byte, http.Header, error) {
	u := m.StoreURL() + p
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return nil, nil, ErrStoreUnavailable
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", "AnasakiStudio/"+CoreVersion)
	resp, err := storeClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	if int64(len(b)) > limit {
		return nil, nil, pkgErr("store answer too large")
	}
	if resp.StatusCode >= 400 {
		return nil, nil, &StoreError{Status: resp.StatusCode, Body: b}
	}
	return b, resp.Header, nil
}

// Store answer shapes that gain installed/installedVersion/updateAvailable.
const (
	StoreRaw      = ""         // passed through untouched
	StoreList     = "list"     // {items: [Summary], total, page}
	StoreDetail   = "detail"   // Summary/Detail object
	StoreRankings = "rankings" // {topDownloads, topRated, trending}
)

// StoreJSON proxies a JSON store endpoint, augmenting summaries with the
// local install state. extraHeader is forwarded (e.g. X-Forwarded-For).
func (m *Manager) StoreJSON(method, p string, q url.Values, body []byte, kind string, extra http.Header) (any, error) {
	hdr := http.Header{}
	for k, v := range extra {
		hdr[k] = v
	}
	if body != nil {
		hdr.Set("Content-Type", "application/json")
	}
	hdr.Set("Accept", "application/json")
	b, _, err := m.storeDo(method, p, q, body, hdr, 8<<20)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON", ErrStoreUnavailable)
	}
	switch kind {
	case StoreList:
		if o, ok := v.(map[string]any); ok {
			m.augmentList(o["items"])
		} else {
			m.augmentList(v)
		}
	case StoreDetail:
		m.augment(v)
	case StoreRankings:
		if o, ok := v.(map[string]any); ok {
			for _, l := range o {
				m.augmentList(l)
			}
		}
	}
	return v, nil
}

func (m *Manager) augmentList(v any) {
	if l, ok := v.([]any); ok {
		for _, it := range l {
			m.augment(it)
		}
	}
}

func (m *Manager) augment(v any) {
	o, ok := v.(map[string]any)
	if !ok {
		return
	}
	id, _ := o["id"].(string)
	inst := m.InstalledVersion(id)
	latest, _ := o["version"].(string)
	o["installed"] = inst != ""
	o["installedVersion"] = inst
	o["updateAvailable"] = inst != "" && latest != "" && CompareVersions(inst, latest) < 0
	// audit H-2: the store's publisher differs from the one the installed
	// plugin is bound to — the id may have changed hands; no update offered
	if bound := m.InstalledPublisher(id); inst != "" && bound != "" {
		if pub, _ := o["publisher"].(string); pub != bound {
			o["publisherChanged"] = true
			o["installedPublisher"] = bound
			o["updateAvailable"] = false
		}
	}
	base := m.StoreURL()
	for _, k := range []string{"iconUrl"} {
		if s, ok := o[k].(string); ok && strings.HasPrefix(s, "/") {
			o[k] = base + s
		}
	}
	if shots, ok := o["screenshots"].([]any); ok {
		for i, s := range shots {
			if str, ok := s.(string); ok && strings.HasPrefix(str, "/") {
				shots[i] = base + str
			}
		}
	}
}

// InstallFromStore downloads id (optionally at version) from the store and
// installs it with source "store".
func (m *Manager) InstallFromStore(id, version string) (Installed, error) {
	return m.InstallFromStoreWith(id, version, InstallOptions{})
}

// InstallFromStoreWith is InstallFromStore with options. Missing
// dependencies come back as *RequiresError whose entries carry the newest
// compatible store version (storeVersion, compatible), so the admin can
// offer "install all" (dependencies first, each with its own consent).
func (m *Manager) InstallFromStoreWith(id, version string, opts InstallOptions) (Installed, error) {
	inst, err := m.installFromStore(id, version, opts)
	var re *RequiresError
	if errors.As(err, &re) {
		m.resolveFromStore(re)
	}
	return inst, err
}

// resolveFromStore looks the missing dependencies up in the store: the
// newest version satisfying the range (or the latest one, compatible=false).
func (m *Manager) resolveFromStore(re *RequiresError) {
	for i := range re.Missing {
		dep := &re.Missing[i]
		b, _, err := m.storeDo(http.MethodGet, "/api/v1/plugins/"+url.PathEscape(dep.ID), nil, nil,
			http.Header{"Accept": {"application/json"}}, 4<<20)
		if err != nil {
			continue
		}
		var d struct {
			Version  string `json:"version"`
			Versions []struct {
				Version string `json:"version"`
			} `json:"versions"`
		}
		if json.Unmarshal(b, &d) != nil {
			continue
		}
		best := ""
		cands := []string{d.Version}
		for _, v := range d.Versions {
			cands = append(cands, v.Version)
		}
		for _, v := range cands {
			if v != "" && SatisfiesRange(v, dep.Range) && (best == "" || CompareVersions(v, best) > 0) {
				best = v
			}
		}
		ok := best != ""
		if !ok {
			best = d.Version
		}
		if best != "" {
			dep.StoreVersion, dep.Latest = best, best
			dep.Compatible = &ok
		}
	}
}

func (m *Manager) installFromStore(id, version string, opts InstallOptions) (Installed, error) {
	b, pub, err := m.DownloadFromStoreMeta(id, version)
	if err != nil {
		return Installed{}, err
	}
	opts.Publisher = pub
	return m.InstallPackageWith(b, "store", id, opts)
}

// DownloadFromStore fetches the zip of id (optionally at version) from the
// store without installing it (the caller validates and installs it).
func (m *Manager) DownloadFromStore(id, version string) ([]byte, error) {
	b, _, err := m.DownloadFromStoreMeta(id, version)
	return b, err
}

// DownloadFromStoreMeta is DownloadFromStore that also returns the store's
// publisher id of the plugin (X-Anasaki-Publisher; "" from an older store),
// for InstallOptions.Publisher.
func (m *Manager) DownloadFromStoreMeta(id, version string) ([]byte, string, error) {
	if !pkgIDRe.MatchString(id) {
		return nil, "", pkgErr("invalid plugin id")
	}
	q := url.Values{}
	if version != "" {
		q.Set("version", version)
	}
	b, hdr, err := m.storeDo(http.MethodGet, "/api/v1/plugins/"+url.PathEscape(id)+"/download", q, nil,
		http.Header{"Accept": {"application/zip"}}, MaxPackageSize)
	if err != nil {
		var pe *PackageError
		if errors.As(err, &pe) {
			return nil, "", pkgErr("package is larger than %d MB", MaxPackageSize>>20)
		}
		return nil, "", err
	}
	// the store's sha256 of the zip (store 1.4.0, security assessment S2): a package
	// changed or cut on the way is refused (an older store sends none)
	if want := strings.ToLower(strings.TrimSpace(hdr.Get("X-Anasaki-SHA256"))); want != "" {
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != want {
			return nil, "", pkgErr("the downloaded package does not match the store's checksum (sha256) — try again")
		}
	}
	return b, strings.TrimSpace(hdr.Get("X-Anasaki-Publisher")), nil
}
