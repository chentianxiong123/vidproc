//go:build android

package main

// Android 后端监听:app 沙箱内 127.0.0.1 本地单用户
// (WebView 同源加载 http://127.0.0.1:端口,与桌面端一致)。

import (
	"log"
	"net/http"
)

// listenBackend 127.0.0.1 单端口起后端
func listenBackend(h http.Handler) {
	addr := "127.0.0.1:" + backendPort()
	if err := http.ListenAndServe(addr, logRequest(h)); err != nil {
		log.Printf("[backend] %v", err)
	}
}
