package ops

import (
	"errors"
	"flag"
	"fmt"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "scale",
		Description: "Resize video (presets: 720p, 1080p, 4k, or specify -h/-w)",
		Usage:       "scale -i <input> -o <output> [-preset 720p|1080p|4k] [-h <height>] [-w <width>]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("scale", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("preset", "", "preset: 720p, 1080p, 4k")
			fs.String("h", "", "target height (maintains aspect ratio)")
			fs.String("w", "", "target width (maintains aspect ratio)")
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
				return errors.New("scale: -i <input> -o <output> required")
			}
			preset := fs.Lookup("preset").Value.String()
			h := fs.Lookup("h").Value.String()
			w := fs.Lookup("w").Value.String()
			if preset == "" && h == "" && w == "" {
				return errors.New("scale: specify -preset, -h, or -w")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			var filter string
			switch preset {
			case "720p":
				filter = "scale=-2:720"
			case "1080p":
				filter = "scale=-2:1080"
			case "4k":
				filter = "scale=-2:2160"
			case "":
				// Custom height/width
				if h != "" && w != "" {
					filter = fmt.Sprintf("scale=%s:%s", w, h)
				} else if h != "" {
					filter = fmt.Sprintf("scale=-2:%s", h)
				} else {
					filter = fmt.Sprintf("scale=%s:-2", w)
				}
			default:
				return fmt.Errorf("scale: unknown preset %s (use 720p, 1080p, 4k)", preset)
			}

			ffargs := []string{"-i", in, "-vf", filter, "-c:a", "copy", "-y", out}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}