package ops

import (
	"errors"
	"flag"
	"fmt"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "flip",
		Description: "Mirror video horizontally or vertically",
		Usage:       "flip -i <input> -o <output> -dir <h|v>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("flip", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("dir", "", "flip direction: h (horizontal/mirror) or v (vertical)")
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
			dir := fs.Lookup("dir").Value.String()
			if in == "" || out == "" {
				return errors.New("flip: -i <input> -o <output> required")
			}
			if dir == "" {
				return errors.New("flip: -dir <h|v> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			var filter string
			switch dir {
			case "h":
				filter = "hflip"
			case "v":
				filter = "vflip"
			default:
				return fmt.Errorf("flip: unknown direction %s (use h or v)", dir)
			}

			ffargs := []string{"-i", in, "-vf", filter, "-c:a", "copy", "-y", out}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
