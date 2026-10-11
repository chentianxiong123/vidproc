package ops

import (
	"errors"
	"flag"
	"fmt"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "cpuenc",
		Description: "CPU transcode with x264/x265 (presets: fast, balanced, high, best)",
		Usage:       "cpuenc -i <input> -o <output> [-preset fast|balanced|high|best] [-codec h264|hevc]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("cpuenc", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("preset", "balanced", "preset: fast, balanced, high, best")
			fs.String("codec", "h264", "video codec: h264, hevc")
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
				return errors.New("cpuenc: -i <input> -o <output> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			preset := fs.Lookup("preset").Value.String()
			codec := fs.Lookup("codec").Value.String()
			var x264Preset, crf string

			switch preset {
			case "fast":
				x264Preset, crf = "veryfast", "25"
			case "balanced":
				x264Preset, crf = "medium", "23"
			case "high":
				x264Preset, crf = "slow", "20"
			case "best":
				x264Preset, crf = "slower", "18"
			default:
				return fmt.Errorf("cpuenc: unknown preset %s (use fast, balanced, high, best)", preset)
			}

			var vcodec string
			switch codec {
			case "h264":
				vcodec = "libx264"
			case "hevc":
				vcodec = "libx265"
			default:
				return fmt.Errorf("cpuenc: unknown codec %s", codec)
			}

			ffargs := []string{
				"-i", in,
				"-c:v", vcodec,
				"-crf", crf,
				"-preset", x264Preset,
				"-tune", "zerolatency",
				"-pix_fmt", "yuv420p",
				"-c:a", "aac",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
