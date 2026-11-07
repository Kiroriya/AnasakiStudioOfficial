package site

import (
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
)

// Multilingual content (2.5.0). The site's language (option "language") has
// the plain addresses; each extra content language (option
// "content_languages": "en, zh") is served under /<lang>/…, with posts and
// pages whose Lang is that language — translations made from an original
// (TranslationOf). Lists on such a page show posts of its language; <head>
// links the translations (hreflang); the "Language switch" block links them
// for visitors; the site header / footer can have a version per language
// (options site_header_component_<lang>, site_footer_component_<lang>).

// OptContentLanguages lists the extra content languages ("en, ja").
const OptContentLanguages = "content_languages"

// pageLangKey is the language of the page being served, in the request's own
// copy of the options ("" = the site's language).
const pageLangKey = "_page_lang"

var langCodeRe = regexp.MustCompile(`^[a-z]{2}(-[a-z]{2})?$`)

// ContentLanguages are the extra content languages (valid codes, without the
// site's own language, at most 10).
func ContentLanguages(opts map[string]string) []string {
	site := strings.ToLower(orDefault(opts["site_language"], orDefault(opts["language"], "th"))) // a /<lang>/ page's copy keeps the site's in site_language
	var out []string
	for _, c := range strings.Split(opts[OptContentLanguages], ",") {
		c = strings.ToLower(strings.TrimSpace(c))
		if langCodeRe.MatchString(c) && c != site && !containsStr(out, c) && len(out) < 10 {
			out = append(out, c)
		}
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// splitLang takes a content language off the front of path ("en/about" →
// "en", "about"); "" when the path has none.
func splitLang(opts map[string]string, path string) (string, string) {
	first, rest, _ := strings.Cut(path, "/")
	if containsStr(ContentLanguages(opts), first) {
		return first, rest
	}
	return "", path
}

// withPageLang is opts for a page in lang: a copy (the shared options stay
// as they are) whose "language" is lang, so the page says <html lang> and its
// texts in that language.
func withPageLang(opts map[string]string, lang string) map[string]string {
	out := make(map[string]string, len(opts)+2)
	for k, v := range opts {
		out[k] = v
	}
	out[pageLangKey] = lang
	if lang != "" {
		out["site_language"] = orDefault(opts["language"], "th")
		out["language"] = lang
	}
	return out
}

// pageLang is the content language of the page being served ("" = the site's).
func pageLang(opts map[string]string) *string {
	l := opts[pageLangKey]
	return &l
}

// langGate answers for a post found by its address when it is not in the
// page's language: the post's own address (no prefix, or another one), or its
// translation in the page's language; 404 when there is none. false = p fits.
func (s *Site) langGate(w http.ResponseWriter, opts map[string]string, p *model.Post) bool {
	want := opts[pageLangKey]
	if p.Lang == want {
		return false
	}
	to := ""
	if want == "" {
		to = PostURL(p)
	} else if tr := s.translationIn(p, want); tr != nil {
		to = PostURL(tr)
	}
	if to == "" {
		s.notFound(w, opts)
		return true
	}
	w.Header().Set("Location", to)
	w.WriteHeader(http.StatusMovedPermanently)
	return true
}

// translationIn is p's published translation in lang ("" = the original's language), or nil.
func (s *Site) translationIn(p *model.Post, lang string) *model.Post {
	list, err := s.st.Translations(p)
	if err != nil {
		return nil
	}
	for _, q := range list {
		if q.Lang == lang && q.Status == model.StatusPublish && q.Type == p.Type {
			return q
		}
	}
	return nil
}

// langNames are the languages' own names (the switch shows them).
var langNames = map[string]string{"th": "ไทย", "en": "English", "zh": "中文", "ja": "日本語", "ko": "한국어", "vi": "Tiếng Việt",
	"lo": "ລາວ", "my": "မြန်မာ", "km": "ខ្មែរ", "ms": "Bahasa Melayu", "id": "Bahasa Indonesia", "fr": "Français", "de": "Deutsch",
	"es": "Español", "ru": "Русский", "ar": "العربية", "hi": "हिन्दी"}

// LangName is lang's own name ("en" → "English"; the code when unknown).
func LangName(lang string) string {
	if n, ok := langNames[lang]; ok {
		return n
	}
	return strings.ToUpper(lang)
}

// langLinks are the page's versions in every language of the site: its
// translations, else that language's home page. post is the page's post
// (nil on archives); cur its language.
func (s *Site) langLinks(opts map[string]string, post *model.Post) []design.LangLink {
	langs := ContentLanguages(opts)
	if len(langs) == 0 {
		return nil
	}
	site := orDefault(opts["site_language"], orDefault(opts["language"], "th"))
	cur := opts[pageLangKey]
	have := map[string]string{}
	if post != nil {
		if list, err := s.st.Translations(post); err == nil {
			for _, q := range list {
				if q.Status == model.StatusPublish && q.Type == post.Type {
					have[q.Lang] = PostURL(q)
				}
			}
		}
	}
	all := append([]string{""}, langs...)
	out := make([]design.LangLink, 0, len(all))
	for _, l := range all {
		u, ok := have[l]
		if !ok {
			u = "/" + l
		}
		code := l
		if l == "" {
			code = site
		}
		out = append(out, design.LangLink{Lang: code, Name: LangName(code), URL: u, Current: l == cur, Translated: ok})
	}
	return out
}

// hreflangHTML links the translations of the page's post in <head>.
func hreflangHTML(links []design.LangLink, base string) string {
	var b strings.Builder
	for _, l := range links {
		if l.Translated {
			b.WriteString(`<link rel="alternate" hreflang="` + html.EscapeString(l.Lang) + `" href="` + html.EscapeString(base+l.URL) + `">`)
		}
	}
	return b.String()
}

// langComponent is the header / footer component of the page's language
// (site_header_component_<lang>) in place of the site's one, else id.
func langComponent(opts map[string]string, id int64) int64 {
	lang := opts[pageLangKey]
	if lang == "" || id == 0 {
		return id
	}
	for _, k := range []string{"site_header_component", "site_footer_component"} {
		if opts[k] == strconv.FormatInt(id, 10) {
			if v, err := strconv.ParseInt(opts[k+"_"+lang], 10, 64); err == nil && v > 0 {
				return v
			}
		}
	}
	return id
}

// LangLinks gives the language switch its links (design.LangSource).
func (s *Source) LangLinks() []design.LangLink { return s.langs }
