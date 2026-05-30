package serve

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	root := strings.TrimSpace(s.cfg.Server.FrontendDir)
	if root == "" {
		writeError(w, http.StatusNotFound, "not_found", "resource not found", nil)
		return
	}

	assetPath := cleanAssetPath(r.URL.Path)
	if serveStaticFile(w, r, root, assetPath) {
		return
	}
	if strings.Contains(filepath.Base(assetPath), ".") {
		writeError(w, http.StatusNotFound, "not_found", "resource not found", nil)
		return
	}
	if serveStaticFile(w, r, root, "index.html") {
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "resource not found", nil)
}

func cleanAssetPath(urlPath string) string {
	clean := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if clean == "." || clean == "" {
		return "index.html"
	}
	return clean
}

func serveStaticFile(w http.ResponseWriter, r *http.Request, root string, assetPath string) bool {
	filename, ok := safeJoin(root, assetPath)
	if !ok {
		return false
	}
	file, err := os.Open(filename)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	if assetPath == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(assetPath, "_app/immutable/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	return true
}

func safeJoin(root string, assetPath string) (string, bool) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	filename := filepath.Join(rootAbs, filepath.FromSlash(assetPath))
	fileAbs, err := filepath.Abs(filename)
	if err != nil {
		return "", false
	}
	if fileAbs != rootAbs && !strings.HasPrefix(fileAbs, rootAbs+string(filepath.Separator)) {
		return "", false
	}
	return fileAbs, true
}
