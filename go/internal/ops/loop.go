package ops

import (
	"errors"
	"flag"
	"strconv"

	"vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "loop",
		Description: "Loop a video N times into one file",
		Usage:       "loop -i <input> -o <output> -times <n>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("loop", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.Int("times", 1, "number of times to play the clip (default: 1 = no loop)")
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
			times := fs.Lookup("times").Value.(flag.Getter).Get().(int)
			if in == "" || out == "" {
				return errors.New("loop: -i <input> -o <output> required")
			}
			if times < 1 {
				return errors.New("loop: -times must be at least 1")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			// -stream_loop is an input option and must precede -i.
			// It counts additional plays, so N total plays is N-1 loops.
			ffargs := []string{}
			if times > 1 {
				ffargs = append(ffargs, "-stream_loop", strconv.Itoa(times-1))
			}
			ffargs = append(ffargs,
				"-i", in,
				"-c", "copy",
				"-y", out,
			)
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
