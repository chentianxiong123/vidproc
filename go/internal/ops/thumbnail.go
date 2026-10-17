package ops

import (
	"errors"
	"flag"

	"vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "thumbnail",
		Description: "Extract a single frame as an image",
		Usage:       "thumbnail -i <input> -o <img> [-ss <t>] [-w <width>]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("thumbnail", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output image (required, .jpg or .png)")
			fs.String("ss", "0", "seek to this time (seconds)")
			fs.String("w", "", "output width; height scales proportionally (default: original)")
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
			if in == "" || out == "" {
				return errors.New("thumbnail: -i <input> -o <img> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			ffargs := []string{
				"-ss", fs.Lookup("ss").Value.String(),
				"-i", in,
				"-vframes", "1",
			}
			if w != "" {
				// -2 keeps the height at an even value proportional to the width.
				ffargs = append(ffargs, "-vf", "scale="+w+":-2")
			}
			ffargs = append(ffargs, "-q:v", "2", "-y", out)
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
