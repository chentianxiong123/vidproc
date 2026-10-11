package ops

import (
	"errors"
	"flag"
	"fmt"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "nvenc",
		Description: "GPU transcode with NVIDIA NVENC (presets: fast, balanced, high, best)",
		Usage:       "nvenc -i <input> -o <output> [-preset fast|balanced|high|best]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("nvenc", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("preset", "balanced", "preset: fast, balanced, high, best")
			fs.Bool("force", false, "overwrite output")
			return fs
		},
		Execute: func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error {
			if in == "" {
				in = fs.Lookup("i").Value.String()
			}
			if out == "" {
				out = fs.Lookup("o").Value.String()
			}
			if in == "" || out == "" {
				return errors.New("nvenc: -i <input> -o <output> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			preset := fs.Lookup("preset").Value.String()
			var nvencPreset, crf, tune string

			// h264_nvenc takes -preset as p1..p7 and -tune as only hq|ll, so the
			// old names ("ld", "zerolatency", "ssim") failed at option parsing
			// before ffmpeg ever reached the driver check.
			switch preset {
			case "fast":
				nvencPreset, crf, tune = "p2", "28", "ll"
			case "balanced":
				nvencPreset, crf, tune = "p4", "23", "ll"
			case "high":
				nvencPreset, crf, tune = "p6", "20", "hq"
			case "best":
				nvencPreset, crf, tune = "p7", "18", "hq"
			default:
				return fmt.Errorf("nvenc: unknown preset %s (use fast, balanced, high, best)", preset)
			}

			ffargs := []string{
				"-i", in,
				"-c:v", "h264_nvenc",
				"-preset", nvencPreset,
				"-crf", crf,
				"-tune", tune,
				"-pix_fmt", "yuv420p",
				"-c:a", "aac",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
