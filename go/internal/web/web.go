// Package web 前端资源工具箱:所有静态资源与模板 go:embed 内嵌进二进制。
//
// 理念(对齐 HomeCast):无 CDN、无 Node、无构建链——htmx/alpine 全部本地化,
// 一个二进制自带全部页面。模板名 = 文件名(如 project_list.html)。
package web

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed assets
var assetsFS embed.FS

//go:embed templates/*.html
var templatesFS embed.FS

// Assets 静态资源(htmx/alpine)
func Assets() fs.FS {
	return assetsFS
}

// Templates 解析全部模板(页面 + htmx 片段)
func Templates() (*template.Template, error) {
	return template.ParseFS(templatesFS, "templates/*.html")
}

// MustTemplates 解析失败即 panic(模板是启动期资产)
func MustTemplates() *template.Template {
	return template.Must(Templates())
}
