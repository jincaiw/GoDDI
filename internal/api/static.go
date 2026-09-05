package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	goddiassets "github.com/jasonwa/goddi"
)

// staticDirCandidates returns the directories that may contain the built
// frontend (web/dist). The first existing directory wins. This mirrors the
// search pattern used elsewhere in the binary so the application can be run
// from the project root, from inside a packaged install, or from a
// system-wide deployment (e.g. /usr/lib/goddi/web/dist).
func staticDirCandidates() []string {
	candidates := []string{
		"./web/dist",
		"./dist",
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "web", "dist"),
			filepath.Join(exeDir, "..", "web", "dist"),
			filepath.Join(exeDir, "..", "..", "web", "dist"),
		)
	}

	candidates = append(candidates,
		"/usr/lib/goddi/web/dist",
		"/usr/local/lib/goddi/web/dist",
		"/opt/goddi/web/dist",
	)

	return candidates
}

// resolveStaticDir returns the first existing static directory or an empty
// string if none is found.
func resolveStaticDir() string {
	for _, dir := range staticDirCandidates() {
		if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
			return dir
		}
	}
	return ""
}

// newStaticHandler returns an http.Handler that serves the built frontend
// from disk, resolving the static directory via resolveStaticDir. If no
// static directory is found, the handler returns a friendly 404 with a hint
// to build the frontend.
func newStaticHandler() http.Handler {
	if dir := resolveStaticDir(); dir != "" {
		return newStaticHandlerInDir(dir)
	}
	return newStaticHandlerInFS(goddiassets.Web())
}

// newStaticHandlerInDir is the test-friendly core of newStaticHandler. The
// behaviour is identical except the static directory is supplied by the
// caller instead of discovered on disk. An empty dir means "no static
// directory was located" and the handler returns a 404 hint.
func newStaticHandlerInDir(dir string) http.Handler {
	if dir == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("frontend assets not found; run `npm run build` inside ./web"))
		})
	}
	return newStaticHandlerInFS(os.DirFS(dir))
}

func newStaticHandlerInFS(staticFS fs.FS) http.Handler {
	indexHTML, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "frontend assets unavailable", http.StatusNotFound)
		})
	}
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := filepath.ToSlash(filepath.Clean(r.URL.Path))
		if cleanPath == "/" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexHTML)
			return
		}

		rel := strings.TrimPrefix(cleanPath, "/")
		if !fs.ValidPath(rel) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexHTML)
			return
		}

		info, err := fs.Stat(staticFS, rel)
		if err != nil || info.IsDir() {
			// Missing build artifacts must never fall back to index.html.
			// Browsers can otherwise cache HTML under a hashed asset URL and
			// keep serving a blank page after a deployment is repaired.
			if strings.HasPrefix(cleanPath, "/assets/") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexHTML)
			return
		}

		// Add a long cache lifetime for hashed asset files (everything
		// under /assets/) and disable caching for index.html so deploys
		// pick up the new bundle immediately.
		// Entry JS files (index-*.js) use no-cache because Vite's content
		// hash for the entry chunk does not include dynamic-import changes,
		// so the filename stays the same even when lazy-loaded chunks change.
		if strings.HasPrefix(cleanPath, "/assets/") {
			base := filepath.Base(cleanPath)
			if strings.HasPrefix(base, "index-") && strings.HasSuffix(base, ".js") {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
		} else if cleanPath == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}

		fileServer.ServeHTTP(w, r)
	})
}
