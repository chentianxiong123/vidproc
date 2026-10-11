package ffmpeg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"os/exec"
)

// Config holds the paths to ffmpeg and ffprobe.
type Config struct {
	FFmpeg  string
	FFprobe string
}

// NewConfig returns a Config with ffmpeg and ffprobe from PATH.
// If not found, returns an error.
func NewConfig() (*Config, error) {
	cfg := &Config{}
	var err error
	cfg.FFmpeg, err = exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}
	cfg.FFprobe, err = exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("ffprobe not found in PATH: %w", err)
	}
	return cfg, nil
}

// WithFFmpeg sets a custom ffmpeg path.
func (c *Config) WithFFmpeg(path string) *Config {
	c.FFmpeg = path
	return c
}

// WithFFprobe sets a custom ffprobe path.
func (c *Config) WithFFprobe(path string) *Config {
	c.FFprobe = path
	return c
}

// ProbeResult holds the result of probing a file.
type ProbeResult struct {
	Format struct {
		Tags map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType  string `json:"codec_type"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		CodecName  string `json:"codec_name"`
		PixFmt     string `json:"pix_fmt"`
		RFrameRate string `json:"r_frame_rate"`
		SampleRate string `json:"sample_rate"`
		Channels   int    `json:"channels"`
	} `json:"streams"`
}

// StreamInfo holds video/audio stream information.
type StreamInfo struct {
	Width       int
	Height      int
	CodecName   string
	PixFmt      string
	RFrameRate  string
	SampleRate  string
	Channels    int
}

// FileInfo holds the essential information needed for concatenation and patching.
type FileInfo struct {
	Path         string
	Video        *StreamInfo
	Audio        *StreamInfo
	FormatTags   map[string]string
}

// Probe runs ffprobe on the given file and returns the ProbeResult.
func (c *Config) Probe(filePath string) (*ProbeResult, error) {
	cmd := exec.Command(c.FFprobe,
		"-v", "error",
		"-show_entries", "format=tags:stream=codec_type,width,height,codec_name,pix_fmt,r_frame_rate,sample_rate,channels",
		"-of", "json",
		filePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w, output: %s", err, string(out))
	}
	var res ProbeResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ffprobe output: %w", err)
	}
	return &res, nil
}

// FileInfoFromProbe converts a ProbeResult to FileInfo.
func FileInfoFromProbe(pr *ProbeResult) *FileInfo {
	info := &FileInfo{}
	for _, s := range pr.Streams {
		if s.CodecType == "video" && info.Video == nil {
			info.Video = &StreamInfo{
				Width:      s.Width,
				Height:     s.Height,
				CodecName:  s.CodecName,
				PixFmt:     s.PixFmt,
				RFrameRate: s.RFrameRate,
			}
		} else if s.CodecType == "audio" && info.Audio == nil {
			info.Audio = &StreamInfo{
				CodecName:  s.CodecName,
				SampleRate: s.SampleRate,
				Channels:   s.Channels,
			}
		}
	}
	info.FormatTags = pr.Format.Tags
	return info
}

// CheckCompatibility checks if two FileInfo have compatible video/audio streams.
func CheckCompatibility(a, b *FileInfo) error {
	if a.Video == nil || b.Video == nil {
		return errors.New("missing video stream")
	}
	if a.Video.Width != b.Video.Width {
		return fmt.Errorf("width mismatch: %d vs %d", a.Video.Width, b.Video.Width)
	}
	if a.Video.Height != b.Video.Height {
		return fmt.Errorf("height mismatch: %d vs %d", a.Video.Height, b.Video.Height)
	}
	if a.Video.CodecName != b.Video.CodecName {
		return fmt.Errorf("video codec mismatch: %s vs %s", a.Video.CodecName, b.Video.CodecName)
	}
	if a.Video.PixFmt != b.Video.PixFmt {
		return fmt.Errorf("pixel format mismatch: %s vs %s", a.Video.PixFmt, b.Video.PixFmt)
	}
	if a.Audio != nil && b.Audio != nil {
		if a.Audio.CodecName != b.Audio.CodecName {
			return fmt.Errorf("audio codec mismatch: %s vs %s", a.Audio.CodecName, b.Audio.CodecName)
		}
		if a.Audio.Channels != b.Audio.Channels {
			return fmt.Errorf("audio channels mismatch: %d vs %d", a.Audio.Channels, b.Audio.Channels)
		}
	}
	return nil
}

// Concat concatenates multiple input files into a single output file using ffmpeg concat demuxer.
// inputs: list of input file paths
// output: output file path
func (c *Config) Concat(inputs []string, output string) error {
	if len(inputs) < 2 {
		return errors.New("concat requires at least 2 input files")
	}
	// Expand globs
	var allInputs []string
	for _, pattern := range inputs {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("glob failed: %w", err)
		}
		if len(files) == 0 {
			return fmt.Errorf("no files matched pattern: %s", pattern)
		}
		allInputs = append(allInputs, files...)
	}
	sort.Strings(allInputs)

	// Write concat list
	listFile := filepath.Join(os.TempDir(), "vidproc_concat_list.txt")
	f, err := os.Create(listFile)
	if err != nil {
		return fmt.Errorf("failed to create concat list: %w", err)
	}
	defer os.Remove(listFile)
	for _, file := range allInputs {
		fmt.Fprintf(f, "file '%s'\n", file)
	}
	f.Close()

	// Run ffmpeg concat
	cmd := exec.Command(c.FFmpeg,
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy",
		"-y",
		output)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat failed: %w, output: %s", err, string(out))
	}
	return nil
}

// Patch copies metadata from ref to target using ffmpeg -map_metadata,
// then appends non-standard MP4 boxes, and copies mtime.
func (c *Config) Patch(target, ref string) error {
	// Step 1: Inject standard metadata using ffmpeg -map_metadata
	tmp := target + ".tmp.mp4"
	cmd := exec.Command(c.FFmpeg,
		"-i", target,
		"-i", ref,
		"-map", "0:v",
		"-map", "0:a",
		"-map_metadata", "1",
		"-c", "copy",
		"-y",
		tmp)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg metadata injection failed: %w, output: %s", err, string(out))
	}
	// Replace target with tmp (Windows: remove target first to avoid file lock)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing target: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("renaming tmp to target: %w", err)
	}
	// Step 2: Copy timestamps
	refInfo, err := os.Stat(ref)
	if err != nil {
		return fmt.Errorf("statting reference: %w", err)
	}
	if err := os.Chtimes(target, refInfo.ModTime(), refInfo.ModTime()); err != nil {
		return fmt.Errorf("changing times: %w", err)
	}
	return nil
}