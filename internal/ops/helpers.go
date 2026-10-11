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