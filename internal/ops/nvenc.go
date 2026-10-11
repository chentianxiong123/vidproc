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

			switch preset {
			case "fast":
				nvencPreset, crf, tune = "ld", "28", "zerolatency"
			case "balanced":
				nvencPreset, crf, tune = "medium", "23", "zerolatency"
			case "high":
				nvencPreset, crf, tune = "hq", "20", "ssim"
			case "best":
				nvencPreset, crf, tune = "p4", "18", "ssim"
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