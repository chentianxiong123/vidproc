package ops

import (
	"errors"
	"flag"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "fade",
		Description: "Crossfade between two videos",
		Usage:       "fade -i <input1> -o <output> [-d <duration>] [--force] <input2>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("fade", flag.ContinueOnError)
			fs.String("i", "", "first input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("d", "1", "crossfade duration in seconds")
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
				return errors.New("fade: second input file required")
			}
			if in == "" || out == "" {
				return errors.New("fade: -i <input1> -o <output> required")
			}
			input2 := args[0]
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			dur := fs.Lookup("d").Value.String()
			ffargs := []string{
				"-i", in,
				"-i", input2,
				// acrossfade and xfade are filters, not codecs: "-c:a acrossfade"
				// made ffmpeg look up a codec of that name and fail.
				"-filter_complex", "[0:v][1:v]xfade=transition=fade:duration=" + dur +
					":offset=0[v];[0:a][1:a]acrossfade=d=" + dur + "[a]",
				"-map", "[v]",
				"-map", "[a]",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
