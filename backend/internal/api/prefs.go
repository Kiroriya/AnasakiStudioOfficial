package api

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"

	"github.com/anasaki/studio/backend/internal/auth"
)

// 2.5.3: each user's own settings of the admin, kept with the account so they follow the user to every device —
// e.g. "dashboard": the order, visibility and size of the dashboard widgets. Every signed-in user may read and
// write only their own; a value is any JSON up to 16 KB, at most 20 keys.

var prefKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,39}$`)

const (
	prefMaxBytes = 16 << 10
	prefMaxKeys  = 20
)

func (a *API) myPrefs(w http.ResponseWriter, r *http.Request) {
	p, err := a.st.UserPrefs(auth.From(r).ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(p))
}

func (a *API) setMyPref(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !prefKeyRe.MatchString(key) {
		writeErr(w, http.StatusBadRequest, "invalid key")
		return
	}
	var v json.RawMessage
	r.Body = http.MaxBytesReader(w, r.Body, prefMaxBytes)
	if err := json.NewDecoder(r.Body).Decode(&v); err == io.EOF {
		v = json.RawMessage("null") // an empty body (HttpClient sends one for null) removes the setting
	} else if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON (at most 16 KB)")
		return
	}
	id := auth.From(r).ID
	cur, err := a.st.UserPrefs(id)
	if err != nil {
		serverErr(w, err)
		return
	}
	all := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(cur), &all)
	if string(v) == "null" {
		delete(all, key)
	} else {
		if _, ok := all[key]; !ok && len(all) >= prefMaxKeys {
			writeErr(w, http.StatusBadRequest, "too many settings")
			return
		}
		all[key] = v
	}
	out, _ := json.Marshal(all)
	if err := a.st.SetUserPrefs(id, string(out)); err != nil {
		serverErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(out)
}
