package ops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/example/vidproc/internal/ffmpeg"
)

// CheckOutput returns an error if output exists and force is false.
func CheckOutput(force bool, output string) error {
	if !force {
		if _, err := os.Stat(output); err == nil {
			return fmt.Errorf("output %s exists; use --force to overwrite", output)
		}
	}
	return nil
}

// probeDuration returns the container duration of a file in seconds.
func probeDuration(cfg *ffmpeg.Config, path string) (float64, error) {
	out, err := exec.Command(cfg.FFprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" || s == "N/A" {
		return 0, fmt.Errorf("ffprobe: no duration for %s", path)
	}
	d, err := strconv.ParseFloat(s, 64)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("ffprobe: bad duration %q for %s", s, path)
	}
	return d, nil
}

// escapeFilterPath escapes a filesystem path for use as a filter option value.
// Forward slashes are used and the drive-letter colon is escaped.
func escapeFilterPath(p string) string {
	return strings.ReplaceAll(filepath.ToSlash(p), ":", "\\:")
}

// defaultFontFile returns a font path that exists on this platform,
// or "" if none of the known locations are present.
func defaultFontFile() string {
	candidates := map[string][]string{
		"windows": {"C:\\Windows\\Fonts\\arial.ttf", "C:\\Windows\\Fonts\\segoeui.ttf"},
		"darwin":  {"/System/Library/Fonts/Supplemental/Arial.ttf", "/System/Library/Fonts/Helvetica.ttf"},
		"linux": {
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf",
		},
	}
	for _, c := range candidates[runtime.GOOS] {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// atempoChain decomposes a speed multiplier into a chain of atempo filters.
// Each factor stays within [0.5, 2.0], which every ffmpeg build accepts.
func atempoChain(rate float64) []string {
	var chain []string
	for rate > 2.0 {
		chain = append(chain, "atempo=2.0")
		rate /= 2.0
	}
	for rate < 0.5 {
		chain = append(chain, "atempo=0.5")
		rate /= 0.5
	}
	if rate > 0 {
		chain = append(chain, "atempo="+strconv.FormatFloat(rate, 'g', -1, 64))
	}
	return chain
}

// drawtextOpts holds the parameters for building a drawtext filter.
type drawtextOpts struct {
	TextFile    string // path to a UTF-8 file containing the text
	Font        string // font file path; "" = auto-detect
	Size        string // "" = 32
	Color       string // "" = white
	X, Y        string // "" = centered
	Box         bool
	BoxColor    string // "" = black@0.5
	BoxPad      string // "" = 10 (maps to boxborderw)
	Border      string // "" = no outline (maps to borderw)
	BorderColor string // "" = black
	Enable      string // time expression, e.g. "between(t,1,2)"; "" = always on
}

// drawtextFilter builds a complete drawtext filtergraph string.
func drawtextFilter(d drawtextOpts) string {
	font := d.Font
	if font == "" {
		font = defaultFontFile()
	}
	size := d.Size
	if size == "" {
		size = "32"
	}
	color := d.Color
	if color == "" {
		color = "white"
	}
	x := d.X
	if x == "" {
		x = "(w-text_w)/2"
	}
	y := d.Y
	if y == "" {
		y = "(h-text_h)/2"
	}
	boxColor := d.BoxColor
	if boxColor == "" {
		boxColor = "black@0.5"
	}
	boxPad := d.BoxPad
	if boxPad == "" {
		boxPad = "10"
	}
	borderColor := d.BorderColor
	if borderColor == "" {
		borderColor = "black"
	}

	// Text goes through a file so arbitrary content needs no escaping.
	// expansion=none stops drawtext interpreting %{...} sequences.
	opts := []string{
		"fontfile='" + escapeFilterPath(font) + "'",
		"textfile='" + escapeFilterPath(d.TextFile) + "'",
		"expansion=none",
		"fontsize='" + size + "'",
		"fontcolor='" + color + "'",
		"x='" + x + "'",
		"y='" + y + "'",
	}
	if d.Box {
		opts = append(opts, "box=1", "boxcolor='"+boxColor+"'", "boxborderw='"+boxPad+"'")
	}
	if d.Border != "" {
		opts = append(opts, "borderw='"+d.Border+"'", "bordercolor='"+borderColor+"'")
	}
	if d.Enable != "" {
		opts = append(opts, "enable='"+d.Enable+"'")
	}
	return "drawtext=" + strings.Join(opts, ":")
}

// writeTempText writes text to a temp file and returns its path.
// Caller must os.Remove the returned path.
func writeTempText(text string) (string, error) {
	tf, err := os.CreateTemp("", "vidproc-drawtext-*.txt")
	if err != nil {
		return "", err
	}
	name := tf.Name()
	if _, err := tf.WriteString(text); err != nil {
		tf.Close()
		os.Remove(name)
		return "", err
	}
	if err := tf.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// DryRunFlag returns true if the global --dry-run flag is set.
// The pipeline engine sets this via env var VIDPROC_DRY_RUN=1.
func DryRunFlag() bool {
	return os.Getenv("VIDPROC_DRY_RUN") == "1"
}

// RunFFmpeg executes ffmpeg with the given args.
// If dryRun is true, prints the command and returns nil.
func RunFFmpeg(cfg *ffmpeg.Config, args []string, dryRun bool) error {
	if dryRun {
		fmt.Fprintf(os.Stderr, "[dry-run] %s %s\n", cfg.FFmpeg, strings.Join(args, " "))
		return nil
	}
	cmd := exec.Command(cfg.FFmpeg, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// MakeTempDir creates a temp directory for pipeline intermediate files.
func MakeTempDir() (string, error) {
	return os.MkdirTemp("", "vidproc-pipeline-*")
}

// CleanupTempDir removes a temp directory and its contents.
func CleanupTempDir(path string) {
	os.RemoveAll(path)
}

// stringSliceFlag is a repeatable string flag.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}
