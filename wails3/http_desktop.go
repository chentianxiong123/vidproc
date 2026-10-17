//go:build !android

package main

// 桌面端（Linux/Windows）后端监听：127.0.0.1 本地单用户
// （WebView 也走它：http://127.0.0.1:端口，同源非 asset 桥）

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