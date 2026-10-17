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
	"strconv"
	"strings"
	"time"

	"vidproc/internal/ffmpeg"
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
			writeErr(w, 404, "该时刻无视频 clip(空画布)")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		jpeg, err := extractFrame(ctx, target.Source, target.SrcTime, FrameOptions{Width: width})
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(jpeg)
	}
}