package plugin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Package format v1 limits (docs/PLATFORM_SPEC.md §1).
const (
	MaxPackageSize   = 30 << 20 // zipped
	MaxPackageFiles  = 500
	MaxFileSize      = 10 << 20 // one file, uncompressed
	MaxWasmSize      = 40 << 20 // a .wasm module, uncompressed
	MaxUnpackedSize  = 80 << 20 // all files, uncompressed
	manifestFileName = "manifest.json"
)

// CoreVersion is the CMS version a manifest's minCore is compared with. It
// follows the release version stamped into the build (SetCoreVersion); local
// and development builds keep this default.
var CoreVersion = "1.0.0"

// SetCoreVersion makes v the CoreVersion when it is a release version
// (semver); "dev" and other non-semver build stamps are ignored.
func SetCoreVersion(v string) bool {
	if !semverRe.MatchString(v) {
		return false
	}
	CoreVersion = v
	return true
}

// AllowedExt lists the file extensions a package may contain.
var AllowedExt = map[string]bool{".json": true, ".html": true, ".css": true, ".js": true, ".md": true, ".txt": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true, ".woff": true, ".woff2": true,
	".ttf": true, ".otf": true, // phase 6: fonts read with site.asset (e.g. Thai PDF output)
	".wasm": true}

// StoreCategories are the plugin store categories (§3).
var StoreCategories = []string{"marketing", "seo", "forms", "ecommerce", "content", "social", "analytics",
	"design", "utilities", "security"}

// Permissions a manifest may request.
var knownPermissions = map[string]bool{"head": true, "bodyEnd": true, "blocks": true, "settings": true, "webhooks": true,
	"service": true, "code": true}

var (
	pkgIDRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,39}$`)
	blockTypeRe = regexp.MustCompile(`^[a-z0-9-]{2,40}$`)
	semverRe    = regexp.MustCompile(`^(\d{1,6})\.(\d{1,6})\.(\d{1,6})(?:-([0-9A-Za-z.-]{1,40}))?(?:\+[0-9A-Za-z.-]{1,40})?$`)
	eventRe     = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,63}$`)
)

// OfficialAuthor is the manifest author of official plugins (the Store's
// "official" flag is the other half of the check).
const OfficialAuthor = "Anasaki Studio"

// ValidPackageID reports whether id is a valid package id.
func ValidPackageID(id string) bool { return pkgIDRe.MatchString(id) }

// Manifest is a package plugin's manifest.json.
type Manifest struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Author      string          `json:"author"`
	Icon        string          `json:"icon"`
	Category    string          `json:"category"`
	Tags        []string        `json:"tags"`
	License     string          `json:"license"`
	Homepage    string          `json:"homepage"`
	MinCore     string          `json:"minCore"`
	Permissions []string        `json:"permissions"`
	Settings    []Field         `json:"settings"`
	Head        []string        `json:"head"`
	BodyEnd     string          `json:"bodyEnd"`
	Blocks      []ManifestBlock `json:"blocks"`
	Webhooks    []Webhook       `json:"webhooks"`
	// Service declares a server program that runs next to the CMS (its own
	// container) and handles routes, blocks, collections and events — see
	// service.go and docs/plugins.md §2. Needs the "service" permission.
	Service *ManifestService `json:"service,omitempty"`
	// Runtime "js": server-side JavaScript/TypeScript run by the Site Code
	// engine (jscode.go, docs/plugins.md §4). Needs the "code" permission
	// and the Code section.
	Runtime string        `json:"runtime,omitempty"`
	Code    *ManifestCode `json:"code,omitempty"`
	// Requires names other plugins (id → semver range) that must be
	// installed (and active, to activate this one).
	Requires map[string]string `json:"requires,omitempty"`
	// OptionalRequires names plugins this one works with when they are
	// installed (id → semver range): never blocks an install; an installed
	// but incompatible version is only a warning (phase 6).
	OptionalRequires map[string]string `json:"optionalRequires,omitempty"`
	// Legacy names the former built-in (Go) plugin whose data this package
	// takes over on upgrade (official plugins, phase 7): its old tables are
	// streamed to the package's code.exports "legacyImport". Must equal id.
	Legacy string `json:"legacy,omitempty"`
	// Capabilities are what the plugin lets roles do (since 2.0.6): the core
	// registers each as "<id>.<capability id>" and the administrator ticks
	// which role holds it in Users → Roles. See capabilities.go.
	Capabilities []ManifestCapability `json:"capabilities,omitempty"`
}

// ManifestCapability is one capability a plugin declares (since 2.0.6).
type ManifestCapability struct {
	ID          string `json:"id"`
	Label       I18n   `json:"label"`
	Description *I18n  `json:"description,omitempty"`
	// Default lists the roles granted the capability when it first appears
	// (install or upgrade); later changes are the administrator's.
	Default []string `json:"default,omitempty"`
}

// ManifestBlock declares a designer block of a package.
type ManifestBlock struct {
	Type     string  `json:"type"`
	Name     I18n    `json:"name"`
	Icon     string  `json:"icon"`
	Template string  `json:"template"`
	CSS      string  `json:"css,omitempty"`
	Props    []Field `json:"props"`
	// Render "service": the plugin's service renders the block (no template).
	Render string `json:"render,omitempty"`
}

// Webhook is an event the package wants POSTed to a URL.
type Webhook struct {
	Event string `json:"event"`
	URL   string `json:"url"`
}

// PackageError is a user-facing validation error (HTTP 400).
type PackageError struct{ Msg string }

func (e *PackageError) Error() string { return e.Msg }

func pkgErr(format string, a ...any) error { return &PackageError{Msg: fmt.Sprintf(format, a...)} }

// HasPermission reports whether the manifest requested p.
func (mf *Manifest) HasPermission(p string) bool {
	for _, q := range mf.Permissions {
		if q == p {
			return true
		}
	}
	return false
}

// IconPath returns the package-relative icon path, "" for an emoji icon.
func (mf *Manifest) IconPath() string {
	if ext := strings.ToLower(path.Ext(mf.Icon)); ext != "" && AllowedExt[ext] && strings.Contains(mf.Icon, ".") &&
		!strings.ContainsAny(mf.Icon, " ") {
		return mf.Icon
	}
	return ""
}

// ParsePackage validates a package zip and returns its manifest and files
// (paths relative to the package root, '/'-separated).
func ParsePackage(data []byte) (*Manifest, map[string][]byte, error) {
	if len(data) > MaxPackageSize {
		return nil, nil, pkgErr("package is larger than %d MB", MaxPackageSize>>20)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, pkgErr("not a valid zip file")
	}
	if len(zr.File) > MaxPackageFiles {
		return nil, nil, pkgErr("too many files (max %d)", MaxPackageFiles)
	}
	files := map[string][]byte{}
	lower := map[string]bool{}
	var total int64
	for _, f := range zr.File {
		// Windows tools (PowerShell 5 Compress-Archive) write '\' separators;
		// normalise first, then apply the same zip-slip checks.
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasSuffix(name, "/") && f.FileInfo().IsDir() {
			if err := checkZipPath(strings.TrimSuffix(name, "/")); err != nil {
				return nil, nil, err
			}
			continue
		}
		if err := checkZipPath(name); err != nil {
			return nil, nil, err
		}
		mode := f.Mode()
		if mode.Type() != 0 { // symlink or any non-regular file
			return nil, nil, pkgErr("%s: only regular files are allowed (no symlinks)", name)
		}
		ext := strings.ToLower(path.Ext(name))
		if !AllowedExt[ext] && !((ext == ".ts" || ext == ".mjs") && isCodeFile(stripTop(name))) {
			return nil, nil, pkgErr("%s: file type %q is not allowed", name, ext)
		}
		limit := int64(MaxFileSize)
		if ext == ".wasm" {
			limit = MaxWasmSize
		}
		if f.UncompressedSize64 > uint64(limit) {
			return nil, nil, pkgErr("%s: file is larger than %d MB", name, limit>>20)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, pkgErr("%s: %v", name, err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, limit+1))
		rc.Close()
		if err != nil {
			return nil, nil, pkgErr("%s: corrupt file", name)
		}
		if int64(len(b)) > limit {
			return nil, nil, pkgErr("%s: file is larger than %d MB", name, limit>>20)
		}
		total += int64(len(b))
		if total > MaxUnpackedSize {
			return nil, nil, pkgErr("package is larger than %d MB unpacked", MaxUnpackedSize>>20)
		}
		if lower[strings.ToLower(name)] {
			return nil, nil, pkgErr("%s: duplicate file name", name)
		}
		lower[strings.ToLower(name)] = true
		files[name] = b
	}
	files = stripCommonRoot(files)
	raw, ok := files[manifestFileName]
	if !ok {
		return nil, nil, pkgErr("manifest.json not found at the package root")
	}
	var mf Manifest
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")) // UTF-8 BOM (Notepad / PowerShell on Windows)
	if err := json.Unmarshal(raw, &mf); err != nil {
		return nil, nil, pkgErr("manifest.json: %v", err)
	}
	if err := mf.Validate(files); err != nil {
		return nil, nil, err
	}
	return &mf, files, nil
}

// checkZipPath rejects absolute paths, drive letters, backslashes, "." and
// ".." segments (zip-slip) and control characters.
func checkZipPath(name string) error {
	if name == "" || len(name) > 200 || !utf8.ValidString(name) {
		return pkgErr("invalid file name %q", name)
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, ":") {
		return pkgErr("unsafe file path %q", name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return pkgErr("unsafe file path %q", name)
		}
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return pkgErr("unsafe file path %q", name)
		}
	}
	return nil
}

// stripCommonRoot handles zips made by "compress folder": when every file
// sits in one top-level directory that holds manifest.json, drop it.
func stripCommonRoot(files map[string][]byte) map[string][]byte {
	if _, ok := files[manifestFileName]; ok || len(files) == 0 {
		return files
	}
	root := ""
	for name := range files {
		top, _, ok := strings.Cut(name, "/")
		if !ok || (root != "" && top != root) {
			return files
		}
		root = top
	}
	if _, ok := files[root+"/"+manifestFileName]; !ok {
		return files
	}
	out := make(map[string][]byte, len(files))
	for name, b := range files {
		out[strings.TrimPrefix(name, root+"/")] = b
	}
	return out
}

// Validate checks the manifest against the format and the package files.
func (mf *Manifest) Validate(files map[string][]byte) error {
	if !pkgIDRe.MatchString(mf.ID) {
		return pkgErr("manifest: id must match ^[a-z0-9][a-z0-9-]{2,39}$")
	}
	if strings.TrimSpace(mf.Name) == "" || utf8.RuneCountInString(mf.Name) > 100 {
		return pkgErr("manifest: name is required (max 100 characters)")
	}
	if !semverRe.MatchString(mf.Version) {
		return pkgErr("manifest: version must be semver (e.g. 1.0.0)")
	}
	if mf.Legacy != "" && mf.Legacy != mf.ID {
		return pkgErr("manifest: legacy must be the plugin's own id (%q)", mf.ID)
	}
	if utf8.RuneCountInString(mf.Description) > 1000 || len(mf.Author) > 200 || len(mf.License) > 100 {
		return pkgErr("manifest: description/author/license too long")
	}
	if mf.Homepage != "" && !(strings.HasPrefix(mf.Homepage, "https://") || strings.HasPrefix(mf.Homepage, "http://")) {
		return pkgErr("manifest: homepage must be an http(s) URL")
	}
	if mf.Category != "" {
		ok := false
		for _, c := range StoreCategories {
			ok = ok || c == mf.Category
		}
		if !ok {
			return pkgErr("manifest: unknown category %q", mf.Category)
		}
	}
	if len(mf.Tags) > 20 {
		return pkgErr("manifest: too many tags")
	}
	if mf.MinCore != "" {
		if !semverRe.MatchString(mf.MinCore) {
			return pkgErr("manifest: minCore must be semver")
		}
		if CompareVersions(mf.MinCore, CoreVersion) > 0 {
			return pkgErr("this plugin needs Anasaki Studio %s or newer", mf.MinCore)
		}
	}
	for _, p := range mf.Permissions {
		if !knownPermissions[p] {
			return pkgErr("manifest: unknown permission %q", p)
		}
	}
	if ip := mf.IconPath(); ip != "" {
		if _, ok := files[ip]; !ok {
			return pkgErr("manifest: icon file %q not found", ip)
		}
	} else if utf8.RuneCountInString(mf.Icon) > 8 {
		return pkgErr("manifest: icon must be an emoji or an image path")
	}
	need := func(perm, what string) error {
		if !mf.HasPermission(perm) {
			return pkgErr("manifest: %s requires the %q permission", what, perm)
		}
		return nil
	}
	exists := func(p string, exts ...string) error {
		if err := checkZipPath(p); err != nil {
			return err
		}
		ext := strings.ToLower(path.Ext(p))
		okExt := false
		for _, e := range exts {
			okExt = okExt || ext == e
		}
		if !okExt {
			return pkgErr("manifest: %q must be one of %v", p, exts)
		}
		if _, ok := files[p]; !ok {
			return pkgErr("manifest: file %q not found in the package", p)
		}
		return nil
	}
	if len(mf.Settings) > 0 {
		if err := need("settings", "settings"); err != nil {
			return err
		}
		if err := ValidateSchema(mf.Settings); err != nil {
			return pkgErr("manifest: settings: %v", err)
		}
	}
	if len(mf.Head) > 0 {
		if err := need("head", "head"); err != nil {
			return err
		}
		if len(mf.Head) > 20 {
			return pkgErr("manifest: too many head assets")
		}
		for _, h := range mf.Head {
			if err := exists(h, ".css", ".js"); err != nil {
				return err
			}
		}
	}
	if mf.BodyEnd != "" {
		if err := need("bodyEnd", "bodyEnd"); err != nil {
			return err
		}
		if err := exists(mf.BodyEnd, ".html"); err != nil {
			return err
		}
	}
	if len(mf.Blocks) > 0 {
		if err := need("blocks", "blocks"); err != nil {
			return err
		}
		if len(mf.Blocks) > 30 {
			return pkgErr("manifest: too many blocks (max 30)")
		}
		seen := map[string]bool{}
		for _, b := range mf.Blocks {
			if !blockTypeRe.MatchString(b.Type) || seen[b.Type] {
				return pkgErr("manifest: block type %q is invalid or duplicated", b.Type)
			}
			seen[b.Type] = true
			if b.Render == "service" {
				if err := need("service", "a service-rendered block"); err != nil {
					return err
				}
				if mf.Service == nil {
					return pkgErr("manifest: block %s is rendered by the service but there is no \"service\" section", b.Type)
				}
			} else if b.Render != "" {
				return pkgErr("manifest: block %s: render must be \"service\" or empty", b.Type)
			} else if err := exists(b.Template, ".html"); err != nil {
				return err
			}
			if b.CSS != "" {
				if err := exists(b.CSS, ".css"); err != nil {
					return err
				}
			}
			if err := ValidateSchema(b.Props); err != nil {
				return pkgErr("manifest: block %s props: %v", b.Type, err)
			}
		}
	}
	if len(mf.Webhooks) > 0 {
		if err := need("webhooks", "webhooks"); err != nil {
			return err
		}
		if len(mf.Webhooks) > 10 {
			return pkgErr("manifest: too many webhooks (max 10)")
		}
		for _, h := range mf.Webhooks {
			if !eventRe.MatchString(h.Event) || strings.TrimSpace(h.URL) == "" || len(h.URL) > 500 {
				return pkgErr("manifest: webhook needs an event and a url")
			}
		}
	}
	if mf.Service != nil {
		if err := need("service", "service"); err != nil {
			return err
		}
		if err := mf.Service.validate(); err != nil {
			return err
		}
		if mf.Service.Runtime == "wasm" {
			if err := exists(mf.Service.Module, ".wasm"); err != nil {
				return err
			}
			if b := files[mf.Service.Module]; len(b) < 8 || string(b[:4]) != "\x00asm" {
				return pkgErr("manifest: %s is not a WebAssembly module", mf.Service.Module)
			}
		}
	} else if mf.HasPermission("service") {
		return pkgErr("manifest: the \"service\" permission needs a \"service\" section")
	}
	if err := mf.validateCode(files); err != nil {
		return err
	}
	if err := mf.validateCapabilities(); err != nil {
		return err
	}
	return mf.validateRequires()
}

// stripTop drops the first path segment when the zip has one top folder
// (the code area check runs before stripCommonRoot).
func stripTop(name string) string {
	if isCodeFile(name) {
		return name
	}
	_, rest, ok := strings.Cut(name, "/")
	if ok {
		return rest
	}
	return name
}

// CompareVersions compares two semver strings (-1, 0, 1). Pre-releases sort
// before the release; unparsable versions compare as strings.
func CompareVersions(a, b string) int {
	ma, mb := semverRe.FindStringSubmatch(a), semverRe.FindStringSubmatch(b)
	if ma == nil || mb == nil {
		return strings.Compare(a, b)
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(ma[i])
		y, _ := strconv.Atoi(mb[i])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case ma[4] == mb[4]:
		return 0
	case ma[4] == "":
		return 1
	case mb[4] == "":
		return -1
	}
	return strings.Compare(ma[4], mb[4])
}
