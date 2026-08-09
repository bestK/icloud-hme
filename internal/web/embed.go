// Package web 把 Vue 3 管理面板的编译产物嵌入到二进制里。
//
// Dockerfile 里 web-builder 阶段会把 web/dist 拷到 internal/web/dist,
// 后端启动时 //go:embed 直接读到内存,不依赖运行时文件系统。
//
// 挂载策略:
//   - /assets/*   → 从 dist 里返对应文件
//   - /favicon.svg → 同上
//   - /           → 返 index.html
//   - /其他非 /api/* 路径 → SPA fallback 到 index.html
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var distFS embed.FS

// FS 返回嵌入的 dist 子文件系统。build 缺失(未运行 npm build)时返回 nil。
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil
	}
	// 探测 index.html 是否存在,不存在说明前端未构建
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}

// Attach 把静态资源和 SPA fallback 挂到 gin 引擎上。
// 已经存在的 /api/* 路由不会被覆盖。
func Attach(r *gin.Engine) {
	f := FS()
	if f == nil {
		return
	}
	httpFS := http.FS(f)

	// 静态资源:直接透传
	r.GET("/assets/*filepath", func(c *gin.Context) {
		http.StripPrefix("/", http.FileServer(httpFS)).ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/favicon.svg", func(c *gin.Context) {
		serveFile(c, httpFS, "favicon.svg")
	})
	r.GET("/", func(c *gin.Context) { serveIndex(c, f) })

	// SPA fallback:非 /api 前缀的 404 都返 index.html
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "not found"})
			return
		}
		serveIndex(c, f)
	})
}

func serveFile(c *gin.Context, httpFS http.FileSystem, name string) {
	f, err := httpFS.Open(name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	http.ServeContent(c.Writer, c.Request, name, st.ModTime(), f.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	}))
}

func serveIndex(c *gin.Context, f fs.FS) {
	raw, err := fs.ReadFile(f, "index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "index.html missing")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", raw)
}
