package ops

import (
	"fmt"
	"os"
	"os/exec"
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