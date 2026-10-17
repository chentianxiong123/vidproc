package ops

import (
	"errors"
	"flag"
	"fmt"

	"vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "mark",
		Description: "Add watermark/logo to video",
		Usage:       "mark -i <input> -o <output> [-pos top|bottom|left|right|center] [-margin <n>] [--force] <logo.png>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("mark", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("pos", "top", "watermark position: top, bottom, left, right, center")
			fs.Int("margin", 10, "margin from edge in pixels")
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
			args := fs.Args()
			if len(args) < 1 {
				return errors.New("mark: logo image required")
			}
			if in == "" || out == "" {
				return errors.New("mark: -i <input> -o <output> required")
			}
			logo := args[0]
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			pos := fs.Lookup("pos").Value.String()
			margin := fs.Lookup("margin").Value.String()
			var overlay string

			switch pos {
			case "top":
				overlay = fmt.Sprintf("overlay=0:%s", margin)
			case "bottom":
				overlay = fmt.Sprintf("overlay=0:H-h-%s", margin)
			case "left":
				overlay = fmt.Sprintf("overlay=%s:0", margin)
			case "right":
				overlay = fmt.Sprintf("overlay=W-w-%s:0", margin)
			case "center":
				overlay = "overlay=(W-w)/2:(H-h)/2"
			default:
				return fmt.Errorf("mark: unknown position %s", pos)
			}

			ffargs := []string{
				"-i", in,
				"-i", logo,
				"-filter_complex", overlay,
				"-c:a", "copy",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
