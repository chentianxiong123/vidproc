// vidproc 统一壳（wails3 v3）：一个入口，三端构建
//
//	🐧 Linux / 🪟 Windows / 🤖 安卓 共用：
//	  - 内嵌 go/ 后端（server.New() handler，模板/静态/API/SQLite 全在内）
//	  - wails WebView 窗口直载 htmx 界面：URL 指向后端真实 HTTP 端口（同源，非 wails asset 桥）
//	  - 桌面端（Linux/Windows）：后端监听 127.0.0.1
//	平台差异全部走同目录 build tag 文件：
//	  ensurehome_other.go —— 数据目录（桌面 ~/.config/vidproc）
//	  http_desktop.go     —— 后端监听地址
//
//	wails 只做系统壳，业务走真实 HTTP（对齐 HomeCast 迁移模式）。
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"vidproc/internal/server"
)

// httpLog 把每个请求的状态/panic 写进文件（日志可取证）
var httpLog string

var app *application.App

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func logRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		defer func() {
			var msg string
			if p := recover(); p != nil {
				msg = fmt.Sprintf("PANIC %s %s -> %v\n%s\n", r.Method, r.URL.Path, p, debug.Stack())
				sw.WriteHeader(500)
			} else {
				msg = fmt.Sprintf("%s %s -> %d (%s)\n", r.Method, r.URL.Path, sw.status, time.Since(start))
			}
			f, err := os.OpenFile(httpLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				f.WriteString(msg)
				f.Close()
			}
		}()
		h.ServeHTTP(sw, r)
	})
}

// backendPort 独立 HTTP 端口；VIDPROC_PORT 覆盖
func backendPort() string {
	if p := os.Getenv("VIDPROC_PORT"); p != "" {
		return p
	}
	return server.DefaultPort
}

func main() {
	httpLog = filepath.Join(vidprocDataPath(), "vidproc-http.log")
	if err := os.MkdirAll(vidprocDataPath(), 0o755); err == nil {
		os.WriteFile(httpLog, []byte("VIDPROC wails3 shell\n"), 0644)
	}

	h := server.New(vidprocDataPath())

	// 桌面壳绑定(wails v3 Services):JS 侧 window.go.main.App.<Method>()
	db, dbErr := server.OpenDB(vidprocDataPath())
	if dbErr != nil {
		log.Printf("[shell] open db: %v", dbErr)
		db = nil
	}
	appServices := []application.Service{application.NewService(newApp(db))}

	// 桌面端：独立端口起后端
	go listenBackend(h)

	app = application.New(application.Options{
		Name:        "vidproc",
		Description: "vidproc studio — lossless concat · filtergraph engine · UDTA",
		Services:    appServices,
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "vidproc studio",
		Width:            1280,
		Height:           800,
		MinWidth:         920,
		MinHeight:        620,
		BackgroundColour: application.NewRGB(248, 249, 250),
		URL:              "http://127.0.0.1:" + backendPort(),
	})

	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}