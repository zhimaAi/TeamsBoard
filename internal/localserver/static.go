package localserver

import (
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"goteams-client"
)

// registerWebUI provides front-end build products embedded during compile time, and falls back to index.html for the front-end history route.
// Fallback to disk web/dist when the frontend is not embedded (development mode) (usually does not exist when provided by Vite, the browser accesses Vite directly).
func registerWebUI(r *gin.Engine) {
	webFS, diskRoot := resolveWebRoot()
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		if webFS != nil {
			serveEmbedded(c, webFS)
			return
		}
		if diskRoot != "" {
			serveDisk(c, diskRoot)
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "未找到 web/dist，请先执行 npm run build 或打开发前端",
		})
	})
}

// resolveWebRoot returns the embedded file system (first) and disk web/dist path (fallback). Returns (nil, "") if both are empty.
func resolveWebRoot() (fs.FS, string) {
	// Prioritize using compile-time embedded front-ends
	if sub, err := fs.Sub(assets.WebFS, "web/dist"); err == nil {
		if _, statErr := fs.Stat(sub, "index.html"); statErr == nil {
			return sub, ""
		}
	}

	// Fallback: web/dist on disk in development mode
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "web", "dist"),
			filepath.Join(filepath.Dir(exe), "dist"),
		)
	}
	candidates = append(candidates, filepath.Join("web", "dist"))
	for _, candidate := range candidates {
		indexPath := filepath.Join(candidate, "index.html")
		if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
			if abs, absErr := filepath.Abs(candidate); absErr == nil {
				return nil, abs
			}
			return nil, candidate
		}
	}
	return nil, ""
}

// serveEmbedded serves static files from the embedded file system, and the SPA route falls back to index.html
func serveEmbedded(c *gin.Context, webFS fs.FS) {
	rel := strings.TrimPrefix(c.Request.URL.Path, "/")
	if rel == "" {
		rel = "index.html"
	}
	// Normalize forward slash paths used by embedded FS and prevent directory traversal
	clean := path.Clean(rel)
	if strings.HasPrefix(clean, "..") || filepath.Separator != '/' && strings.Contains(clean, string(filepath.Separator)) {
		c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
		return
	}

	data, err := fs.ReadFile(webFS, clean)
	if err != nil {
		// Only the front-end history route without extension falls back to index.html.
		// If static resources are missing, 404 must be returned; if index.html is disguised as JS/CSS,
		// The browser will only report "Unexpected token '<'", masking the real lack of resources.
		if path.Ext(clean) != "" {
			c.Status(http.StatusNotFound)
			return
		}
		data, err = fs.ReadFile(webFS, "index.html")
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "未找到 web/dist"})
			return
		}
		c.Data(http.StatusOK, mime.TypeByExtension(".html"), data)
		return
	}
	c.Data(http.StatusOK, mime.TypeByExtension(filepath.Ext(clean)), data)
}

// serveDisk serves static files from disk web/dist
func serveDisk(c *gin.Context, webRoot string) {
	// Explicitly block directory traversal: if it jumps out of webRoot or becomes an absolute path after normalization, it will be rejected.
	// Consistent with the defense level of serveEmbedded.
	relativePath := strings.TrimPrefix(filepath.Clean(c.Request.URL.Path), string(filepath.Separator))
	clean := filepath.Clean(relativePath)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
		return
	}
	candidate := filepath.Join(webRoot, relativePath)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		c.File(candidate)
		return
	}
	if filepath.Ext(relativePath) != "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(filepath.Join(webRoot, "index.html"))
}
