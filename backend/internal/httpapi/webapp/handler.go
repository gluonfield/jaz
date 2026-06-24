package webapp

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func New(dir string) http.Handler {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	return Handler{Dir: dir}
}

type Handler struct {
	Dir string
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !PublicRequest(r) {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.Abs(h.Dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	if clean != "/" {
		if target, ok := safeFile(root, clean); ok {
			info, err := os.Stat(target)
			if err == nil && !info.IsDir() {
				http.ServeFile(w, r, target)
				return
			}
		}
		if strings.HasPrefix(clean, "/assets/") || path.Ext(clean) != "" {
			http.NotFound(w, r)
			return
		}
	}
	http.ServeFile(w, r, filepath.Join(root, "index.html"))
}

func PublicRequest(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && !ReservedPath(r.URL.Path)
}

func ReservedPath(raw string) bool {
	p := path.Clean("/" + raw)
	if p == "/health" {
		return true
	}
	for _, prefix := range []string{"/v1", "/mcp", "/jazmem"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

func safeFile(root, cleanURLPath string) (string, bool) {
	target := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanURLPath, "/")))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return target, true
}
