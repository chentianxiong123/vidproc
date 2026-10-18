// preview.go — GET /api/projects/{id}/frame?t=3.5
// 画布预览:对播放头所在时刻的视频 clip 抽一帧 JPEG。
// 纯函数帧提取,不动时间轴;前端视口用 <img> 直接加载。
package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vidproc/internal/ffmpeg"
	"vidproc/internal/media"
	"vidproc/internal/timeline/spec"
)

// FrameOptions 抽帧参数。
type FrameOptions struct {
	// Width 输出宽(0 = 原尺寸)。
	Width int
}

// extractFrame 对单个素材在源内时刻 srcTime(秒)抽一帧 JPEG。
// srcTime 已含 trim 偏移;这里只负责拼 ffmpeg 命令,不做业务换算。
// 返回 JPEG 字节。素材不存在/解码失败 → error(前端显示灰底)。
func extractFrame(ctx context.Context, sourcePath string, srcTime float64, opts FrameOptions) ([]byte, error) {
	cfg, err := ffmpeg.NewConfig()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg 不可用: %w", err)
	}
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", strconv.FormatFloat(srcTime, 'f', 3, 64),
		"-i", sourcePath,
		"-frames:v", "1",
	}
	if opts.Width > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:-2", opts.Width))
	}
	args = append(args, "-f", "mjpeg", "-")

	cmd := exec.CommandContext(ctx, cfg.FFmpeg, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("抽帧失败 %q @%.3fs: %w", sourcePath, srcTime, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("抽帧为空 %q @%.3fs", sourcePath, srcTime)
	}
	return out, nil
}

// frameTarget 播放头时刻的抽帧目标:最上层可抽帧 clip(视频/overlay 轨,
// 后声明的在上层——对齐 vidonex z-index 语义)。
type frameTarget struct {
	Source  string
	SrcTime float64
}

// resolveFrameTarget 从解析后的 spec 找 t 时刻最上层视频 clip,
// 换算源内抽帧时刻:trim + (t - start) * speed(速度在源中前进更快)。
func resolveFrameTarget(vs *spec.VideoSpec, t float64) *frameTarget {
	for i := len(vs.Tracks) - 1; i >= 0; i-- {
		tr := vs.Tracks[i]
		if tr.Kind != "video" && tr.Kind != "overlay" {
			continue
		}
		for _, c := range tr.Clips {
			start := parseSeconds(c.Start)
			dur := parseSeconds(c.Duration)
			if dur <= 0 || t < start || t >= start+dur {
				continue
			}
			trim := parseSeconds(c.Trim)
			speed := c.Speed
			if speed == 0 {
				speed = 1
			}
			return &frameTarget{
				Source:  c.Source,
				SrcTime: trim + (t-start)*speed,
			}
		}
	}
	return nil
}

// SourceFrameHandler GET /api/projects/{id}/source-frame?src=<path>&t=0&w=240
// 对 spec 中引用的任意源文件抽封面帧(对齐上游 ClipItem 直接用 clip.source 显示封面)。
// 安全:src 必须命中该项目 spec 的某个 clip.source(白名单),只回 JPEG 帧/图片原图,不泄露任意文件。
func SourceFrameHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		vs, err := spec.ParseYAML(strings.NewReader(p.SpecYAML))
		if err != nil {
			writeErr(w, 400, "spec 解析失败: "+err.Error())
			return
		}

		src := r.URL.Query().Get("src")
		if src == "" {
			writeErr(w, 400, "src 必填")
			return
		}
		// 白名单:必须等于某个 clip.source(精确匹配,防任意文件抽帧)
		allowed := false
		for _, tr := range vs.Tracks {
			for _, c := range tr.Clips {
				if c.Source == src {
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

		t := 0.0
		if raw := r.URL.Query().Get("t"); raw != "" {
			if f, err := strconv.ParseFloat(raw, 64); err == nil && f >= 0 {
				t = f
			}
		}
		width := 0
		if raw := r.URL.Query().Get("w"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				width = n
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// 图片源:直接回原图(不抽帧)
		switch strings.ToLower(filepath.Ext(src)) {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp":
			http.ServeFile(w, r, src)
			return
		}
		// 源文件可能已不存在 → 用素材库同名副本恢复(画布预览/封面统一行为)
		actual := media.ResolveSource(db, p.ID, src)
		jpeg, err := extractFrame(ctx, actual, t, FrameOptions{Width: width})
		if err != nil {
			// 源丢失且无素材副本:204 静默(缩略图空,不污染控制台)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(jpeg)
	}
}

// FrameHandler GET /api/projects/{id}/frame?t=3.5&w=960 → 播放头帧 JPEG。
func FrameHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		vs, err := spec.ParseYAML(strings.NewReader(p.SpecYAML))
		if err != nil {
			writeErr(w, 400, "spec 解析失败: "+err.Error())
			return
		}

		t := 0.0
		if raw := r.URL.Query().Get("t"); raw != "" {
			if f, err := strconv.ParseFloat(raw, 64); err == nil && f >= 0 {
				t = f
			}
		}
		width := 0
		if raw := r.URL.Query().Get("w"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				width = n
			}
		}

		target := resolveFrameTarget(vs, t)
		if target == nil {
			// 空画布/无视频 clip:204 静默(前端显示空态,不污染控制台)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		// 源文件可能已不存在 → 用素材库同名副本恢复画面
		actual := media.ResolveSource(db, p.ID, target.Source)
		jpeg, err := extractFrame(ctx, actual, target.SrcTime, FrameOptions{Width: width})
		if err != nil {
			// 抽帧失败(源丢失且无素材副本):204 静默
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(jpeg)
	}
}