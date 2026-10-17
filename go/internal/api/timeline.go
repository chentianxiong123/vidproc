// timeline.go — GET /api/projects/{id}/timeline
// 把项目 spec 解析成结构化时间轴 JSON(数值化秒,不做任何执行),
// 供前端真实渲染 clip 块(宽 = 时长 × 像素/秒)。对齐 vidonex TimelineContainer。
package api

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"vidproc/internal/timeline/spec"
)

// ClipJSON 数值化的时间轴 clip(前端渲染用)。
type ClipJSON struct {
	ID       string  `json:"id"`
	Source   string  `json:"source"`
	Name     string  `json:"name"` // 源文件名(时间轴/预览显示)
	Start    float64 `json:"start"`     // 时间轴起始秒
	Duration float64 `json:"duration"`  // 时长秒
	Trim     float64 `json:"trim"`      // 源内裁剪起点秒
	Speed    float64 `json:"speed"`
	HasAudio bool    `json:"has_audio"`
	Kind     string  `json:"kind"` // 所属轨道类型,前端配色用
}

// TrackJSON 数值化的轨道。
type TrackJSON struct {
	ID    string     `json:"id"`
	Kind  string     `json:"kind"`
	Clips []ClipJSON `json:"clips"`
}

// TimelineJSON 时间轴整体:画布 + 轨道 + 总时长。
type TimelineJSON struct {
	Width    int         `json:"width"`
	Height   int         `json:"height"`
	FPS      float64     `json:"fps"`
	Duration float64     `json:"duration"` // 最大 clip 末端秒
	Tracks   []TrackJSON `json:"tracks"`
}

// parseSeconds 统一把 spec 的 any 时长(数字 / "3.3s" / "00:00:03.300")转成秒。
// 解析失败记 0(前端以 0 时长 clip 跳过,渲染不崩)。
func parseSeconds(raw any) float64 {
	d, err := spec.ParseDurationValue(raw)
	if err != nil {
		return 0
	}
	return d.Seconds()
}

// specToTimelineJSON 纯函数:VideoSpec → TimelineJSON。api 层唯一业务解析点,
// 不触碰 DB/渲染;可单测。
func specToTimelineJSON(vs *spec.VideoSpec) TimelineJSON {
	tl := TimelineJSON{
		Width:  vs.Canvas.Width,
		Height: vs.Canvas.Height,
		FPS:    specFPS(vs.FPS),
		Tracks: make([]TrackJSON, 0, len(vs.Tracks)),
	}
	if tl.Width == 0 {
		tl.Width = 1920
	}
	if tl.Height == 0 {
		tl.Height = 1080
	}

	var maxEnd float64
	for _, tr := range vs.Tracks {
		tj := TrackJSON{ID: tr.ID, Kind: tr.Kind, Clips: make([]ClipJSON, 0, len(tr.Clips))}
		for _, c := range tr.Clips {
			start := parseSeconds(c.Start)
			dur := parseSeconds(c.Duration)
			if dur <= 0 {
				continue // 无时长 clip 不渲染(引擎同样会拒绝)
			}
			trim := parseSeconds(c.Trim)
			speed := c.Speed
			if speed == 0 {
				speed = 1
			}
			hasAudio := true
			if c.HasAudio != nil {
				hasAudio = *c.HasAudio
			}
			tj.Clips = append(tj.Clips, ClipJSON{
				ID:       c.ID,
				Source:   c.Source,
				Name:     filepath.Base(strings.ReplaceAll(c.Source, "\\", "/")),
				Start:    start,
				Duration: dur,
				Trim:     trim,
				Speed:    speed,
				HasAudio: hasAudio,
				Kind:     tr.Kind,
			})
			if end := start + dur; end > maxEnd {
				maxEnd = end
			}
		}
		tl.Tracks = append(tl.Tracks, tj)
	}
	tl.Duration = maxEnd
	return tl
}

// specFPS 解析 fps 字段(数字 / "30" / "30fps")。
func specFPS(raw any) float64 {
	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return v
		}
	case int:
		if v > 0 {
			return float64(v)
		}
	case string:
		s := strings.TrimSpace(strings.TrimSuffix(v, "fps"))
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			return f
		}
	}
	return 30
}

// TimelineHandler GET /api/projects/{id}/timeline → 数值化时间轴 JSON。
// 挂在 Router 上(见 api.go),解析项目 spec、不执行任何 ffmpeg。
func TimelineHandler(db *sql.DB) http.HandlerFunc {
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
		writeJSON(w, 200, specToTimelineJSON(vs))
	}
}
