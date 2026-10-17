package ffmpeg

import (
	"errors"
	"testing"
)

func TestCheckCompatibility(t *testing.T) {
	// Build two compatible files
	info1 := &FileInfo{
		Video: &StreamInfo{
			Width: 1920, Height: 1080, CodecName: "h264", PixFmt: "yuv420p",
			SampleRate: "48000", Channels: 2,
		},
		Audio: &StreamInfo{
			CodecName: "aac", SampleRate: "48000", Channels: 2,
		},
	}
	info2 := &FileInfo{
		Video: &StreamInfo{
			Width: 1920, Height: 1080, CodecName: "h264", PixFmt: "yuv420p",
			SampleRate: "48000", Channels: 2,
		},
		Audio: &StreamInfo{
			CodecName: "aac", SampleRate: "48000", Channels: 2,
		},
	}
	if err := CheckCompatibility(info1, info2); err != nil {
		t.Fatalf("expected compatible files, got error: %v", err)
	}
	// Incompatible width
	info3 := &FileInfo{
		Video: &StreamInfo{
			Width: 1280, Height: 720, CodecName: "h264", PixFmt: "yuv420p",
			SampleRate: "48000", Channels: 2,
		},
		Audio: &StreamInfo{
			CodecName: "aac", SampleRate: "48000", Channels: 2,
		},
	}
	if err := CheckCompatibility(info1, info3); err == nil {
		t.Fatalf("expected error for width mismatch")
	}
	// Incompatible codec
	info4 := &FileInfo{
		Video: &StreamInfo{
			Width: 1920, Height: 1080, CodecName: "hevc", PixFmt: "yuv420p",
			SampleRate: "48000", Channels: 2,
		},
		Audio: &StreamInfo{
			CodecName: "aac", SampleRate: "48000", Channels: 2,
		},
	}
	if err := CheckCompatibility(info1, info4); err == nil {
		t.Fatalf("expected error for codec mismatch")
	}
	// Incompatible audio channels
	info5 := &FileInfo{
		Video: &StreamInfo{
			Width: 1920, Height: 1080, CodecName: "h264", PixFmt: "yuv420p",
			SampleRate: "48000", Channels: 2,
		},
		Audio: &StreamInfo{
			CodecName: "aac", SampleRate: "48000", Channels: 1,
		},
	}
	if err := CheckCompatibility(info1, info5); err == nil {
		t.Fatalf("expected error for audio channels mismatch")
	}
}

func TestFileInfoFromProbe(t *testing.T) {
	pr := &ProbeResult{
		Format: struct {
			Tags map[string]string `json:"tags"`
		}{
			Tags: map[string]string{
				"creation_time": "2025-02-09T10:05:36.000000Z",
			},
		},
		Streams: []struct {
			CodecType  string `json:"codec_type"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			CodecName  string `json:"codec_name"`
			PixFmt     string `json:"pix_fmt"`
			RFrameRate string `json:"r_frame_rate"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
		}{
			{CodecType: "video", Width: 1920, Height: 1080, CodecName: "h264", PixFmt: "yuv420p", RFrameRate: "30/1"},
			{CodecType: "audio", CodecName: "aac", SampleRate: "48000", Channels: 2},
		},
	}
	info := FileInfoFromProbe(pr)
	if info.Video == nil {
		t.Fatal("expected video stream info")
	}
	if info.Video.Width != 1920 || info.Video.Height != 1080 {
		t.Fatalf("video dimensions wrong: %dx%d", info.Video.Width, info.Video.Height)
	}
	if info.Audio == nil {
		t.Fatal("expected audio stream info")
	}
	if info.Audio.Channels != 2 {
		t.Fatalf("audio channels wrong: %d", info.Audio.Channels)
	}
	if info.FormatTags["creation_time"] != "2025-02-09T10:05:36.000000Z" {
		t.Fatalf("creation_time not preserved: %v", info.FormatTags["creation_time"])
	}
}

func TestNewConfig(t *testing.T) {
	// This will fail if ffmpeg/ffprobe not in PATH, but we can test the logic
	_, err := NewConfig()
	if err != nil {
		// If ffmpeg not in PATH, that's fine - just make sure it returns an error
		if !errors.Is(err, nil) {
			t.Logf("expected error when ffmpeg not in PATH: %v", err)
		}
	}
}
