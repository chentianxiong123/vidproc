package api

import (
	"database/sql"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"vidproc/internal/media"
	"vidproc/internal/timeline/spec"
)

// MediaFileHandler GET /api/projects/{id}/media?src=<path>
// 媒体直服端点:浏览器音频/视频元素播放源(对齐参考 resolveMediaUrl → /api/media/files)。
// 安全:src 必须精确命中项目 spec 的某个 clip.source(白名单,防任意文件泄露);
// 源丢失时用素材库同名副本恢复(与 SourceFrameHandler 同一行为);
// http.ServeContent 提供 Range/206 —— 音频 seek/进度条依赖(HTMLAudioElement 必需)。
func MediaFileHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		src := r.URL.Query().Get("src")
		if src == "" {
			writeErr(w, 400, "src 必填")
			return
		}
		vs, err := spec.ParseYAML(strings.NewReader(p.SpecYAML))
		if err != nil {
			writeErr(w, 400, "spec 解析失败: "+err.Error())
			return
		}
		// 白名单:精确等于某个 clip.source(与 SourceFrameHandler 同规则)
		allowed := false
		for _, tr := range vs.Tracks {
			for _, c := range tr.Clips {
				if sameMediaPath(c.Source, src) {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			writeErr(w, 404, "src 不在该项目时间轴白名单内")
			return
		}

		// 源文件可能已不存在 → 用素材库同名副本恢复
		actual := media.ResolveSource(db, p.ID, src)
		f, err := os.Open(actual)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ct := mime.TypeByExtension(filepath.Ext(actual))
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, filepath.Base(actual), st.ModTime(), f)
	}
}

// sameMediaPath 媒体路径等价比较:统一反斜杠→正斜杠 + 大小写折叠(Windows 文件系统大小写不敏感)。
func sameMediaPath(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "\\", "/")) }
	return norm(a) == norm(b)
}
