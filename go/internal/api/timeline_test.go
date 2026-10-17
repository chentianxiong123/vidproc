// timeline_test.go — timeline 数值化纯函数测试(不碰 ffmpeg/DB)。
package api

import (
	"strings"
	"testing"

	"vidproc/internal/timeline/spec"
)

const timelineTestSpec = `
version: "1.0"
canvas: {preset: "1080p", background_color: "black"}
fps: "30"
tracks:
  - id: "v1"
    kind: "video"
    clips:
      - {id: "c1", source: "C:/media/a.mp4", start: "0s", duration: "3.3s"}
      - {id: "c2", source: "C:/media/b.mp4", start: "3.3s", duration: "3s", trim: "0.5s", speed: 2}
      - {id: "bad", source: "C:/media/x.mp4", start: "6.3s"}  # 无 duration,应跳过
  - id: "a1"
    kind: "audio"
    clips:
      - {id: "ac1", source: "C:/media/a.mp4", start: "0s", duration: "3.3s"}
`

func TestSpecToTimelineJSON(t *testing.T) {
	vs, err := spec.ParseYAML(strings.NewReader(timelineTestSpec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tl := specToTimelineJSON(vs)

	if tl.Width != 1920 || tl.Height != 1080 {
		t.Errorf("canvas = %dx%d, want 1920x1080", tl.Width, tl.Height)
	}
	if tl.FPS != 30 {
		t.Errorf("fps = %v, want 30", tl.FPS)
	}
	// 最大末端 = max(3.3, 6.3) = 6.3(坏 clip 无时长不计)
	if tl.Duration != 6.3 {
		t.Errorf("duration = %v, want 6.3", tl.Duration)
	}

	if len(tl.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(tl.Tracks))
	}
	v := tl.Tracks[0]
	if len(v.Clips) != 2 {
		t.Fatalf("video clips = %d, want 2(坏 clip 应跳过)", len(v.Clips))
	}
	c1 := v.Clips[0]
	if c1.Start != 0 || c1.Duration != 3.3 {
		t.Errorf("c1 = %.1f..%.1f, want 0..3.3", c1.Start, c1.Duration)
	}
	if c1.Name != "a.mp4" {
		t.Errorf("c1 name = %q, want a.mp4", c1.Name)
	}
	c2 := v.Clips[1]
	if c2.Start != 3.3 || c2.Duration != 3 || c2.Trim != 0.5 || c2.Speed != 2 {
		t.Errorf("c2 = start %.1f dur %.1f trim %.1f speed %.1f", c2.Start, c2.Duration, c2.Trim, c2.Speed)
	}
	if v.Kind != "video" || tl.Tracks[1].Kind != "audio" {
		t.Errorf("track kinds wrong: %q / %q", v.Kind, tl.Tracks[1].Kind)
	}
}

func TestResolveFrameTarget(t *testing.T) {
	vs, err := spec.ParseYAML(strings.NewReader(timelineTestSpec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// t=1.0 → c1:srcTime = 0 + (1.0-0)*1 = 1.0
	target := resolveFrameTarget(vs, 1.0)
	if target == nil {
		t.Fatal("t=1.0: no target")
	}
	if !strings.Contains(target.Source, "a.mp4") {
		t.Errorf("t=1.0 source = %q, want a.mp4", target.Source)
	}
	almostEqual := func(a, b float64) bool {
		d := a - b
		if d < 0 {
			d = -d
		}
		return d < 1e-6
	}
	if !almostEqual(target.SrcTime, 1.0) {
		t.Errorf("t=1.0 srcTime = %.6f, want 1.00", target.SrcTime)
	}

	// t=4.0 → c2:srcTime = 0.5 + (4.0-3.3)*2 = 1.9
	target = resolveFrameTarget(vs, 4.0)
	if target == nil {
		t.Fatal("t=4.0: no target")
	}
	if !strings.Contains(target.Source, "b.mp4") {
		t.Errorf("t=4.0 source = %q, want b.mp4", target.Source)
	}
	if !almostEqual(target.SrcTime, 1.9) {
		t.Errorf("t=4.0 srcTime = %.6f, want 1.90", target.SrcTime)
	}

	// t=7 → 超出所有 clip,应为 nil
	if target := resolveFrameTarget(vs, 7.0); target != nil {
		t.Errorf("t=7: unexpected target %+v", target)
	}
}

func TestSpecFPS(t *testing.T) {
	cases := map[any]float64{
		"30":    30,
		"29.97": 29.97,
		"60fps": 60,
		float64(24): 24,
		int(25):      25,
		nil:          30, // 默认
		"junk":       30, // 解析失败默认
	}
	for raw, want := range cases {
		if got := specFPS(raw); got != want {
			t.Errorf("specFPS(%v) = %v, want %v", raw, got, want)
		}
	}
}
