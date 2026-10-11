package ops

import (
	"errors"
	"flag"
	"os"

	"github.com/example/vidproc/internal/ffmpeg"
)


func init() {
	Register(&Op{
		Name:        "concat",
		Description: "Concatenate multiple videos (lossless)",
		Usage:       "concat -o <output> [--force] [--keep-mtime] <files...>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("concat", flag.ContinueOnError)
			fs.String("o", "", "output file (required)")
			fs.Bool("force", false, "overwrite output")
			fs.Bool("keep-mtime", false, "keep mtime from first input file")
			return fs
		},
		Execute: func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error {
			if out == "" {
				out = fs.Lookup("o").Value.String()
			}
			if out == "" {
				return errors.New("concat: -o <output> required (flags must precede the file list)")
			}
			inputs := fs.Args()
			for _, a := range inputs {
				if len(a) > 0 && a[0] == '-' {
					return errors.New("concat: flags must come before the file list, e.g. concat -o out.mp4 [--force] a.mp4 b.mp4")
				}
			}
			if len(inputs) < 2 {
				return errors.New("concat: need at least 2 input files")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}
			if err := cfg.Concat(inputs, out); err != nil {
				return err
			}
			if fs.Lookup("keep-mtime").Value.String() == "true" && len(inputs) > 0 {
				fi, err := os.Stat(inputs[0])
				if err != nil {
					return err
				}
				_ = os.Chtimes(out, fi.ModTime(), fi.ModTime())
			}
			return nil
		},
	})
}
