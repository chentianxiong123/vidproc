package ops

import (
	"errors"
	"flag"
	"fmt"

	"vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "crop",
		Description: "Crop video to a region",
		Usage:       "crop -i <input> -o <output> -w <width> -h <height> [-x <left>] [-y <top>]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("crop", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("w", "", "crop width (required)")
			fs.String("h", "", "crop height (required)")
			fs.String("x", "", "left offset (default: centered)")
			fs.String("y", "", "top offset (default: centered)")
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
			w := fs.Lookup("w").Value.String()
			h := fs.Lookup("h").Value.String()
			x := fs.Lookup("x").Value.String()
			y := fs.Lookup("y").Value.String()
			if in == "" || out == "" {
				return errors.New("crop: -i <input> -o <output> required")
			}
			if w == "" || h == "" {
				return errors.New("crop: -w <width> and -h <height> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}
			// ffmpeg expressions: (iw - ow)/2 centers the crop automatically.
			xf, yf := "(iw-ow)/2", "(ih-oh)/2"
			if x != "" {
				xf = x
			}
			if y != "" {
				yf = y
			}
			filter := fmt.Sprintf("crop=%s:%s:%s:%s", w, h, xf, yf)

			ffargs := []string{"-i", in, "-vf", filter, "-c:a", "copy", "-y", out}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
