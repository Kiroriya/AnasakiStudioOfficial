package site

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Site Code on the public site (phase 3): partial re-renders for the
// client runtime and "call code" actions.
//
//	GET  /_c/{postId}/{nodeId}?<page query>  → {"nodes":{"<id>":"<outerHTML>"},"head":{}}
//	POST /_a/{name}  {input, ctx:{postId, path, query}, update:[ids]}
//	     headers X-CSRF-Token (= cookie anasaki_csrf) and a same-site Origin/Referer
//	     → {message?, redirect?, errors?, data?, nodes:{<id>: html}}

// CSRFCookie is the double-submit cookie of POST /_a (readable by the page).
const CSRFCookie = "anasaki_csrf"

const maxActionBody = 1 << 20

// csrfCookie returns a new CSRF cookie when the request has none.
func csrfCookie(r *http.Request) string {
	if r == nil {
		return ""
	}
	if c, err := r.Cookie(CSRFCookie); err == nil && len(c.Value) >= 32 {
		return ""
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	c := &http.Cookie{Name: CSRFCookie, Value: hex.EncodeToString(b), Path: "/", SameSite: http.SameSiteLaxMode,
		Secure: plugin.RequestScheme(r) == "https"}
	return c.String()
}

func writeJSONResp(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// serveDynamic answers /_c/… and /_a/…; false for other paths.
func (s *Site) serveDynamic(w http.ResponseWriter, r *http.Request, opts map[string]string) bool {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/_c/"):
		s.servePartial(w, r, opts, strings.TrimPrefix(p, "/_c/"))
	case strings.HasPrefix(p, "/_a/"):
		s.serveAction(w, r, opts, strings.TrimPrefix(p, "/_a/"))
	default:
		return false
	}
	return true
}

// designFor resolves what page postId shows: a published page (its own
// design), a published post (the post template, or its own design) or a
// template; ctx is the {{post.*}} context and path the page's address.
func (s *Site) designFor(id int64, opts map[string]string) (root *design.Node, ctx design.PostCtx, path string, ok bool) {
	if id <= 0 {
		return nil, nil, "", false
	}
	p, err := s.st.GetPost(id)
	if err != nil {
		return nil, nil, "", false
	}
	raw := p.Design
	switch p.Type {
	case model.TypePage:
		if p.Status != model.StatusPublish {
			return nil, nil, "", false
		}
		ctx, path = PostContext(p), PostURL(p)
		if home, _ := strconv.ParseInt(opts["home_page_id"], 10, 64); home == p.ID || (home == 0 && p.Slug == "home") {
			ctx["url"], ctx["isHome"], path = "/", "1", "/"
		}
	case model.TypePost:
		if p.Status != model.StatusPublish {
			return nil, nil, "", false
		}
		ctx, path = PostContext(p), PostURL(p)
		tpl := s.template(opts["post_template_id"])
		if tpl == nil && len(p.Design) == 0 {
			tpl = s.defaultPostTemplate()
		}
		if tpl != nil {
			raw = tpl.Design
		}
	case model.TypeTemplate:
	default:
		if typeByID(s.customTypes(opts), p.Type) == nil || p.Status != model.StatusPublish {
			return nil, nil, "", false
		}
		ctx, path = PostContext(p), PostURL(p)
		tpl := s.typeTemplate(opts, p.Type)
		if tpl == nil && len(p.Design) == 0 {
			tpl = s.defaultPostTemplate()
		}
		if tpl != nil {
			raw = tpl.Design
		}
	}
	root, err = design.Parse(raw)
	if err != nil {
		return nil, nil, "", false
	}
	return root, ctx, path, true
}

func cleanPath(p string) string {
	p, _, _ = strings.Cut(p, "?")
	if !design.SameSitePath(p) || len(p) > 500 {
		return ""
	}
	return p
}

// servePartial re-renders one node (a collection and its pagination) of a
// page for the query: GET /_c/{postId}/{nodeId}.
func (s *Site) servePartial(w http.ResponseWriter, r *http.Request, opts map[string]string, rest string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSONResp(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	idStr, nodeID, _ := strings.Cut(rest, "/")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if nodeID == "" || len(nodeID) > 200 {
		writeJSONResp(w, http.StatusNotFound, map[string]string{"error": "ไม่พบส่วนที่ต้องการ"})
		return
	}
	root, ctxPost, path, ok := s.designFor(id, opts)
	if !ok {
		writeJSONResp(w, http.StatusNotFound, map[string]string{"error": "ไม่พบหน้าที่ต้องการ"})
		return
	}
	if hp := cleanPath(r.Header.Get("X-Rea-Path")); hp != "" {
		path = hp
	}
	src := NewSource(s.st, s.plugins)
	var data map[string]any
	sess := s.session(r, opts)
	if sess != nil {
		defer sess.Close()
		src.code = sess
		data = sess.PageData(path)
	}
	dc := designContext(opts, ctxPost, "", path, r.URL.Query(), data)
	if sess != nil {
		dc.Visitor = visitorFunc(sess)
	}
	out := design.RenderFragments(root, dc, src, design.FragmentReq{IDs: []string{nodeID}})
	if sess != nil {
		for _, c := range sess.Cookies() {
			w.Header().Add("Set-Cookie", c)
		}
	}
	writeJSONResp(w, http.StatusOK, map[string]any{"nodes": out.Fragments, "head": map[string]any{}})
}

// sameOrigin checks the Origin (or Referer) of a state-changing request
// against the site's own address.
func sameOrigin(r *http.Request, opts map[string]string) bool {
	allowed := map[string]bool{strings.ToLower(plugin.RequestScheme(r) + "://" + plugin.RequestHost(r)): true}
	if u, err := url.Parse(strings.TrimSpace(opts["site_url"])); err == nil && u.Host != "" {
		allowed[strings.ToLower(u.Scheme+"://"+u.Host)] = true
	}
	check := func(raw string) bool {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return false
		}
		return allowed[strings.ToLower(u.Scheme+"://"+u.Host)]
	}
	if o := r.Header.Get("Origin"); o != "" {
		return o != "null" && check(o)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		return check(ref)
	}
	return false
}

type actionBody struct {
	Input map[string]any `json:"input"`
	Ctx   struct {
		PostID any            `json:"postId"`
		Path   string         `json:"path"`
		Query  map[string]any `json:"query"`
	} `json:"ctx"`
	Update []string `json:"update"`
}

// serveAction runs site.action name: POST /_a/{name}.
func (s *Site) serveAction(w http.ResponseWriter, r *http.Request, opts map[string]string, name string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSONResp(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !design.QualifiedActionRe.MatchString(name) {
		writeJSONResp(w, http.StatusNotFound, map[string]string{"error": "ไม่พบ action นี้"})
		return
	}
	c, err := r.Cookie(CSRFCookie)
	tok := r.Header.Get("X-CSRF-Token")
	if err != nil || c.Value == "" || tok == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(tok)) != 1 {
		writeJSONResp(w, http.StatusForbidden, map[string]string{"error": "CSRF token ไม่ถูกต้อง — โหลดหน้าใหม่แล้วลองอีกครั้ง"})
		return
	}
	if !sameOrigin(r, opts) {
		writeJSONResp(w, http.StatusForbidden, map[string]string{"error": "คำขอต้องมาจากหน้าเว็บนี้เท่านั้น"})
		return
	}
	var body actionBody
	r.Body = http.MaxBytesReader(w, r.Body, maxActionBody)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONResp(w, http.StatusBadRequest, map[string]string{"error": "ข้อมูลที่ส่งมาไม่ถูกต้อง"})
		return
	}
	sess := s.session(r, opts)
	if sess == nil {
		writeJSONResp(w, http.StatusNotFound, map[string]string{"error": "ไม่พบ action นี้"})
		return
	}
	defer sess.Close()
	if body.Input == nil {
		body.Input = map[string]any{}
	}
	res := sess.RunAction(name, body.Input)
	out := map[string]any{}
	if res.Status != http.StatusOK {
		out["error"] = res.Error
		if len(res.Errors) > 0 {
			out["errors"] = res.Errors
		}
		for _, c := range sess.Cookies() {
			w.Header().Add("Set-Cookie", c)
		}
		if res.Status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "60")
		}
		writeJSONResp(w, res.Status, out)
		return
	}
	nodes := map[string]string{}
	if len(body.Update) > 0 || len(res.Update) > 0 || len(res.Refresh) > 0 {
		postID, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(body.Ctx.PostID)), 10, 64)
		if root, ctxPost, path, ok := s.designFor(postID, opts); ok {
			if p := cleanPath(body.Ctx.Path); p != "" {
				path = p
			}
			query := url.Values{}
			for k, v := range body.Ctx.Query {
				switch x := v.(type) {
				case []any:
					for _, it := range x {
						query.Add(k, design.Stringify(it))
					}
				default:
					query.Set(k, design.Stringify(x))
				}
			}
			data := map[string]any{}
			for k, v := range sess.PageData(path) {
				data[k] = v
			}
			for k, v := range res.Page {
				data[k] = v
			}
			srcs := make([]string, 0, len(res.Refresh))
			for _, n := range res.Refresh {
				srcs = append(srcs, "code:"+n)
			}
			src := NewSource(s.st, s.plugins)
			src.code = sess
			dc := designContext(opts, ctxPost, "", path, query, data)
			dc.Visitor = visitorFunc(sess)
			fr := design.RenderFragments(root, dc, src,
				design.FragmentReq{IDs: body.Update, Names: res.Update, Sources: srcs})
			nodes = fr.Fragments
		}
	}
	out["nodes"] = nodes
	if res.Message != "" {
		out["message"] = res.Message
	}
	if res.Redirect != "" {
		out["redirect"] = res.Redirect
	}
	if len(res.Errors) > 0 {
		out["errors"] = res.Errors
	}
	if res.Data != nil {
		out["data"] = res.Data
	}
	for _, c := range sess.Cookies() {
		w.Header().Add("Set-Cookie", c)
	}
	writeJSONResp(w, http.StatusOK, out)
}
