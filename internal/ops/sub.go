package ops

import (
	"errors"
	"flag"
	"fmt"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "sub",
		Description: "Burn subtitles into video",
		Usage:       "sub -i <input> -o <output> [-font <name>] [-size <n>] [--force] <subtitle.srt>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("sub", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("font", "Arial", "font name")
			fs.String("size", "24", "font size")
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
				return errors.New("sub: subtitle file required")
			}
			if in == "" || out == "" {
				return errors.New("sub: -i <input> -o <output> required")
			}
			subtitle := args[0]
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			font := fs.Lookup("font").Value.String()
			size := fs.Lookup("size").Value.String()
			style := fmt.Sprintf("FontName=%s,FontSize=%s", font, size)
			ffargs := []string{
				"-i", in,
				"-vf", "subtitles=" + subtitle + ":force_style='" + style + "'",
				"-c:a", "copy",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}