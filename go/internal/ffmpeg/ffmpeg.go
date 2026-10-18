package ffmpeg

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"vidproc/internal/mp4"
)


// Config holds the paths to ffmpeg and ffprobe.
type Config struct {
	FFmpeg  string
	FFprobe string
}

// NewConfig locates ffmpeg and ffprobe from env vars and PATH.
// Callers that already know the path (e.g. the --ffmpeg flag) should use
// Resolve instead, so a broken PATH cannot mask a working explicit path.
//
//	ffmpeg:  VIDPROC_FFMPEG, else PATH
//	ffprobe: VIDPROC_FFPROBE, else PATH, else the ffprobe next to ffmpeg
func NewConfig() (*Config, error) { return Resolve("", "") }

// Resolve builds a Config, preferring an explicitly supplied path over the env
// var and PATH. Explicit wins because a caller that already knows where ffmpeg
// lives must not need PATH to be correct at all.
//
// The sibling fallback matters because the common Windows builds (gyan.dev,
// BtbN) ship both executables in one bin/. Without it, pinning ffmpeg to an
// older build would silently keep using PATH's newer ffprobe.
func Resolve(explicitFFmpeg, explicitFFprobe string) (*Config, error) {
	ff, err := pick("ffmpeg", "VIDPROC_FFMPEG", explicitFFmpeg, lookPath("ffmpeg"))
	if err != nil {
		return nil, err
	}
	if ff == "" {
		return nil, errors.New("ffmpeg not found: put it on PATH, set VIDPROC_FFMPEG, or pass --ffmpeg <path>")
	}

	fp, err := pick("ffprobe", "VIDPROC_FFPROBE", explicitFFprobe, lookPath("ffprobe"), SiblingProbe(ff))
	if err != nil {
		return nil, err
	}
	if fp == "" {
		return nil, errors.New("ffprobe not found: put it on PATH, set VIDPROC_FFPROBE, or place it next to ffmpeg")
	}

	return &Config{FFmpeg: ff, FFprobe: fp}, nil
}

// pick returns the first candidate that exists, in order: the explicit value,
// then the env var, then the fallbacks (PATH). A value that does not exist is
// an error rather than a fallthrough — a typo must not silently resolve to a
// different version.
func pick(which, envKey, explicit string, fallbacks ...string) (string, error) {
	env := os.Getenv(envKey)
	for _, c := range append([]string{explicit, env}, fallbacks...) {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
		if c == env {
			return "", fmt.Errorf("%s=%s does not exist", envKey, c)
		}
		return "", fmt.Errorf("%s path %s does not exist", which, c)
	}
	return "", nil
}

// lookPath returns the PATH location of exe, or "" if it is not on PATH.
func lookPath(exe string) string {
	if p, err := exec.LookPath(exe); err == nil {
		return p
	}
	return ""
}

// SiblingProbe returns the ffprobe next to ffmpegExe, reusing that file's own
// extension so it matches "ffmpeg.exe" -> "ffprobe.exe" and "ffmpeg" ->
// "ffprobe". Returns "" if no such file exists.
func SiblingProbe(ffmpegExe string) string {
	p := filepath.Join(filepath.Dir(ffmpegExe), "ffprobe"+filepath.Ext(ffmpegExe))
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
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
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
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

// FormatDuration 返回媒体总时长秒(0 = 未能探测)。
func (p *ProbeResult) FormatDuration() float64 {
	if p == nil || p.Format.Duration == "" {
		return 0
	}
	d, err := strconv.ParseFloat(p.Format.Duration, 64)
	if err != nil || d < 0 {
		return 0
	}
	return d
}

// StreamInfo holds video/audio stream information.
type StreamInfo struct {
	Width      int
	Height     int
	CodecName  string
	PixFmt     string
	RFrameRate string
	SampleRate string
	Channels   int
}

// FileInfo holds the essential information needed for concatenation and patching.
type FileInfo struct {
	Path       string
	Video      *StreamInfo
	Audio      *StreamInfo
	FormatTags map[string]string
}

// Probe runs ffprobe on the given file and returns the ProbeResult.
func (c *Config) Probe(filePath string) (*ProbeResult, error) {
	cmd := exec.Command(c.FFprobe,
		"-v", "error",
		"-show_entries", "format=duration:format=tags:stream=codec_type,width,height,codec_name,pix_fmt,r_frame_rate,sample_rate,channels",
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
	// Expand globs.
	var allInputs []string
	for _, pattern := range inputs {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("glob failed: %w", err)
		}
		if len(files) > 0 {
			allInputs = append(allInputs, files...)
			continue
		}
		// Glob only understands the native path syntax of this OS, and treats
		// brackets as character classes. A literal file named e.g. clip[01].mp4
		// matches nothing, so fall back to it as a plain path.
		if _, err := os.Stat(pattern); err == nil {
			allInputs = append(allInputs, pattern)
			continue
		}
		return fmt.Errorf("no files matched pattern: %s", pattern)
	}
	sort.Strings(allInputs)

	// Write concat list. The concat demuxer resolves relative paths in the list
	// relative to the list file's own directory, not the caller's working
	// directory. The list lives in os.TempDir(), so entries must be made
	// absolute here or they resolve one level too high.
	listFile := filepath.Join(os.TempDir(), "vidproc_concat_list.txt")
	f, err := os.Create(listFile)
	if err != nil {
		return fmt.Errorf("failed to create concat list: %s", err)
	}
	defer os.Remove(listFile)
	for _, file := range allInputs {
		abs, err := filepath.Abs(file)
		if err != nil {
			return fmt.Errorf("absolute path for %s: %w", file, err)
		}
		fmt.Fprintf(f, "file '%s'\n", abs)
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
		RemoveEmpty(output)
		return fmt.Errorf("ffmpeg concat failed: %w, output: %s", err, string(out))
	}
	// Insert the first input file's udta box into the output, preserving
	// non-standard MP4 boxes (e.g. xiaomi vendor tags). This runs after
	// the ffmpeg concat succeeds so it won't affect the zero-byte file
	// detection above.
	if len(allInputs) > 0 {
		udtaBox, err := extractFirstInputUDTA(allInputs[0])
		if err == nil {
			if err := mp4.InsertUDTABox(output, udtaBox); err != nil {
				// Non-fatal: the video still plays fine without the udta box.
				_ = err
			}
		}
	}
	return nil
}

// extractFirstInputUDTA extracts the full udta box (header+payload)
// from the first input file's moov/udta, searching recursively.
func extractFirstInputUDTA(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	moov := findMoovRecursive(data)
	if moov == nil {
		return nil, fmt.Errorf("no moov in %s", filePath)
	}
	udta := findUdtaInMoov(moov)
	if udta == nil {
		return nil, fmt.Errorf("no udta in %s", filePath)
	}
	// Rebuild the full udta box (header + payload) since mp4.Box only
	// exposes the payload slice.
	total := 8 + len(udta.Payload)
	buf := make([]byte, 8, total)
	binary.BigEndian.PutUint32(buf, uint32(total))
	copy(buf[4:8], "udta")
	buf = append(buf, udta.Payload...)
	return buf, nil
}

// findMoovRecursive recursively searches for a moov box in the given data.
func findMoovRecursive(data []byte) *mp4.Box {
	boxes := mp4.ScanBoxes(data)
	for _, b := range boxes {
		if b.Type == "moov" {
			return &b
		}
		// Recursively search inside potential container boxes
		if b.Type == "free" || b.Type == "trak" || b.Type == "mdia" || b.Type == "minf" || b.Type == "stbl" || b.Type == "udta" || b.Type == "moof" || b.Type == "mfra" {
			if found := findMoovRecursive(b.Payload); found != nil {
				return found
			}
		}
	}
	return nil
}

// findUdtaInMoov searches for a udta box within a moov box's payload.
func findUdtaInMoov(moov *mp4.Box) *mp4.Box {
	inner := mp4.ScanBoxes(moov.Payload)
	for _, b := range inner {
		if b.Type == "udta" {
			return &b
		}
		// Recursively search inside container boxes within moov
		if b.Type == "trak" || b.Type == "mdia" || b.Type == "minf" || b.Type == "stbl" || b.Type == "udta" {
			if found := findUdtaInMoov(&mp4.Box{
				Type:    b.Type,
				Offset:  b.Offset,
				Size:    b.Size,
				Payload: b.Payload,
			}); found != nil {
				return found
			}
		}
	}
	return nil
}
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
		RemoveEmpty(tmp)
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

// RemoveEmpty deletes path if it exists and is empty. ffmpeg opens the output
// file before the encoder runs, so a hard encoder failure (unsupported preset,
// wrong GPU driver) leaves a zero-byte file that otherwise looks like a
// successful result. Non-empty partial outputs are left alone for inspection.
func RemoveEmpty(path string) {
	if st, err := os.Stat(path); err == nil && st.Size() == 0 {
		os.Remove(path)
	}
}

// Version returns the first line of `ffmpeg -version`(About 系统诊断用)。
func (c *Config) Version() string {
	if c == nil || c.FFmpeg == "" {
		return "不可用"
	}
	out, err := exec.Command(c.FFmpeg, "-version").Output()
	if err != nil {
		return "不可用"
	}
	line := string(out)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if line == "" {
		return "不可用"
	}
	return line
}
