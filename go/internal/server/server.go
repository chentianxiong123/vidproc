// Package server 组装 vidproc 全部 HTTP handler(对齐 HomeCast go/server)。
// 被 cmd/vidproc studio(独立进程)和 desktop(Wails 壳内嵌)复用。
package server

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"vidproc/internal/api"
	"vidproc/internal/hx"
	"vidproc/internal/project"
	"vidproc/internal/store"
	"vidproc/internal/web"
)

// DefaultPort studio 默认端口。
const DefaultPort = "8398"

// New 组装全部 handler(htmx 层 + 静态资产)。
func New(dataDir string) http.Handler {
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".config", "vidproc")
	}
	db, err := store.Open(filepath.Join(dataDir, "vidproc.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	// 组装层收集各域包 schema 传入 store(store 不 import 业务,避免环)
	if err := store.Migrate(db, project.ProjectSchema); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	hxH := hx.New(db)

	outer := http.NewServeMux()
	// JSON API 端口(AI/CLI/脚本) + htmx 端口(浏览器),共享同一域包
	outer.Handle("/api/", api.Router(db))
	// 静态资产(htmx/alpine)挂 /assets;Assets() 的 FS 根是 assets/ 目录,
	// StripPrefix 去掉 /assets/ 后需落在目录内,用 fs.Sub 提升一层。
	assetRoot, _ := fs.Sub(web.Assets(), "assets")
	outer.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetRoot))))
	// htmx 层:页面 + 功能切片
	outer.Handle("/", hxH.Router())
	return withCORS(outer)
}

// ListenAndServe 启动服务(addr 空时用 VIDPROC_PORT 或默认端口)。
func ListenAndServe(addr string) error {
	if addr == "" {
		if p := os.Getenv("VIDPROC_PORT"); p != "" {
			addr = "127.0.0.1:" + p
		} else {
			addr = "127.0.0.1:" + DefaultPort
		}
	}
	handler := New("")
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}
	log.Printf("vidproc studio listening on http://%s", addr)
	return srv.ListenAndServe()
}

// withCORS 允许任意来源(本地单用户服务;桌面壳跨域访问)。
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, HX-Request")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}