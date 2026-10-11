package ops

import (
	"errors"
	"flag"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "concat",
		Description: "Concatenate multiple videos (lossless)",
		Usage:       "concat <files...> -o <output>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("concat", flag.ContinueOnError)
			fs.String("o", "", "output file (required)")
			fs.Bool("force", false, "overwrite output")
			return fs
		},
		Execute: func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error {
			if out == "" {
				out = fs.Lookup("o").Value.String()
			}
			if out == "" {
				return errors.New("concat: -o <output> required")
			}
			inputs := fs.Args()
			if len(inputs) < 2 {
				return errors.New("concat: need at least 2 input files")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}
			return cfg.Concat(inputs, out)
		},
	})
}