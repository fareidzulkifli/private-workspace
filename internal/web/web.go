package web

import (
	"bytes"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	hashedAssetRE   = regexp.MustCompile(`(?:[-.][A-Za-z0-9_-]{8,})\.[A-Za-z0-9]+$`)
	viteAssetRE     = regexp.MustCompile(`^(.+)-[A-Za-z0-9_-]{8,}(\.[A-Za-z0-9]+)$`)
	indexAssetRefRE = regexp.MustCompile(`(?:src|href)="(/assets/[^"]+\.(?:css|js))"`)
)

type Handler struct {
	dist      http.FileSystem
	publicDir string
	favicon   string
	index     []byte
	hasDist   bool
}

type Options struct {
	DistDir   string
	PublicDir string
	Favicon   string
}

func New(opts Options) *Handler {
	if opts.DistDir == "" {
		opts.DistDir = "web/dist"
	}
	if opts.PublicDir == "" {
		opts.PublicDir = "web/public"
	}
	if opts.Favicon == "" {
		opts.Favicon = "web/public/favicon.ico"
	}

	h := &Handler{
		dist:      http.Dir(opts.DistDir),
		publicDir: opts.PublicDir,
		favicon:   opts.Favicon,
		index:     placeholderIndex(),
	}
	if index, err := os.ReadFile(filepath.Join(opts.DistDir, "index.html")); err == nil {
		h.index = index
		h.hasDist = true
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cleanPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	if cleanPath == "/favicon.ico" && h.serveLocalFile(w, r, h.favicon, cleanPath) {
		return
	}
	if h.hasDist && h.serveDistFile(w, r, cleanPath) {
		return
	}
	if h.serveLocalFile(w, r, filepath.Join(h.publicDir, strings.TrimPrefix(cleanPath, "/")), cleanPath) {
		return
	}
	if IsClientRoute(cleanPath) {
		h.serveIndex(w, r)
		return
	}
	if IsConcreteAsset(cleanPath) {
		if h.redirectStaleDistAsset(w, r, cleanPath) {
			return
		}
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

func (h *Handler) serveDistFile(w http.ResponseWriter, r *http.Request, cleanPath string) bool {
	name := strings.TrimPrefix(cleanPath, "/")
	if name == "" {
		name = "index.html"
	}
	file, err := h.dist.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	setCacheControl(w, cleanPath, name == "index.html")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	return true
}

func (h *Handler) serveLocalFile(w http.ResponseWriter, r *http.Request, filename string, requestPath string) bool {
	file, err := os.Open(filename)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	setCacheControl(w, requestPath, false)
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	return true
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	setCacheControl(w, "/index.html", true)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	reader := bytes.NewReader(h.index)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	http.ServeContent(w, r, "index.html", time.Time{}, reader)
}

func (h *Handler) redirectStaleDistAsset(w http.ResponseWriter, r *http.Request, cleanPath string) bool {
	if !h.hasDist || !isHashedAsset(cleanPath) || !strings.HasPrefix(cleanPath, "/assets/") {
		return false
	}
	prefix, ext, ok := splitViteAssetName(path.Base(cleanPath))
	if !ok || prefix != "index" || (ext != ".css" && ext != ".js") {
		return false
	}
	fallback, ok := h.findFallbackAsset(cleanPath)
	if !ok || fallback == cleanPath {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, fallback, http.StatusTemporaryRedirect)
	return true
}

func (h *Handler) findFallbackAsset(requestPath string) (string, bool) {
	prefix, ext, ok := splitViteAssetName(path.Base(requestPath))
	if !ok {
		return "", false
	}
	if prefix == "index" && (ext == ".css" || ext == ".js") {
		if asset, ok := h.currentIndexAsset(ext); ok {
			return asset, true
		}
	}

	dirName := strings.TrimPrefix(path.Dir(requestPath), "/")
	dir, err := h.dist.Open(dirName)
	if err != nil {
		return "", false
	}
	defer dir.Close()

	infos, err := dir.Readdir(-1)
	if err != nil {
		return "", false
	}
	var selected os.FileInfo
	for _, info := range infos {
		if info.IsDir() || info.Name() == path.Base(requestPath) {
			continue
		}
		candidatePrefix, candidateExt, ok := splitViteAssetName(info.Name())
		if !ok || candidatePrefix != prefix || candidateExt != ext {
			continue
		}
		if selected == nil || info.ModTime().After(selected.ModTime()) {
			selected = info
		}
	}
	if selected == nil {
		return "", false
	}
	return "/" + path.Join(dirName, selected.Name()), true
}

func (h *Handler) currentIndexAsset(ext string) (string, bool) {
	for _, match := range indexAssetRefRE.FindAllSubmatch(h.index, -1) {
		asset := string(match[1])
		if path.Ext(asset) == ext && h.hasDistFile(strings.TrimPrefix(asset, "/")) {
			return asset, true
		}
	}
	return "", false
}

func setCacheControl(w http.ResponseWriter, requestPath string, index bool) {
	if index {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	if isHashedAsset(requestPath) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if contentType := mime.TypeByExtension(path.Ext(requestPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
}

func IsConcreteAsset(requestPath string) bool {
	cleanPath := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	if cleanPath == "/favicon.ico" {
		return true
	}
	ext := path.Ext(cleanPath)
	return ext != ""
}

func IsClientRoute(requestPath string) bool {
	cleanPath := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	return cleanPath == "/gitnote" ||
		strings.HasPrefix(cleanPath, "/gitnote/") ||
		cleanPath == "/share" ||
		strings.HasPrefix(cleanPath, "/share/")
}

func (h *Handler) HasConcreteAsset(requestPath string) bool {
	cleanPath := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	if cleanPath == "/favicon.ico" {
		return isRegularFile(h.favicon)
	}
	if h.hasDist {
		name := strings.TrimPrefix(cleanPath, "/")
		if name != "" && h.hasDistFile(name) {
			return true
		}
	}
	return isRegularFile(filepath.Join(h.publicDir, strings.TrimPrefix(cleanPath, "/")))
}

func isHashedAsset(requestPath string) bool {
	base := path.Base(requestPath)
	return strings.HasPrefix(requestPath, "/assets/") && hashedAssetRE.MatchString(base)
}

func splitViteAssetName(name string) (string, string, bool) {
	match := viteAssetRE.FindStringSubmatch(name)
	if len(match) != 3 {
		return "", "", false
	}
	return match[1], match[2], true
}

func (h *Handler) hasDistFile(name string) bool {
	file, err := h.dist.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	return err == nil && !info.IsDir()
}

func isRegularFile(filename string) bool {
	info, err := os.Stat(filename)
	return err == nil && !info.IsDir()
}

func placeholderIndex() []byte {
	return []byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Private Workspace</title>
</head>
<body>
  <main id="root">Private Workspace</main>
</body>
</html>
`)
}
