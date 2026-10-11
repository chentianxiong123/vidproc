package ops

import (
	"errors"
	"flag"
	"fmt"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "rotate",
		Description: "Rotate video by 90/180/270 degrees",
		Usage:       "rotate -i <input> -o <output> -angle <90|180|270>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("rotate", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("angle", "", "rotation degrees clockwise: 90, 180, or 270")
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
			angle := fs.Lookup("angle").Value.String()
			if in == "" || out == "" {
				return errors.New("rotate: -i <input> -o <output> required")
			}
			if angle == "" {
				return errors.New("rotate: -angle <90|180|270> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			// transpose=2 rotates 90 deg clockwise, transpose=1 rotates 90 deg counter-clockwise.
			// 180 degrees is two clockwise rotations.
			var filter string
			switch angle {
			case "90":
				filter = "transpose=2"
			case "180":
				filter = "transpose=2,transpose=2"
			case "270":
				filter = "transpose=1"
			default:
				return fmt.Errorf("rotate: unknown angle %s (use 90, 180, or 270)", angle)
			}

			ffargs := []string{"-i", in, "-vf", filter, "-c:a", "copy", "-y", out}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
