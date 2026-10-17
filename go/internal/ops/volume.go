package ops

import (
	"errors"
	"flag"
	"math"
	"strconv"

	"vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "volume",
		Description: "Adjust audio volume",
		Usage:       "volume -i <input> -o <output> [-level <n>] [-mute]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("volume", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.Float64("level", 1, "volume multiplier: 2 = louder, 0.5 = quieter")
			fs.Bool("mute", false, "silence the audio")
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
			level := fs.Lookup("level").Value.(flag.Getter).Get().(float64)
			mute := fs.Lookup("mute").Value.String() == "true"
			if in == "" || out == "" {
				return errors.New("volume: -i <input> -o <output> required")
			}
			if level < 0 || math.IsInf(level, 0) || math.IsNaN(level) {
				return errors.New("volume: -level must be non-negative")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			// level=0 is identical to mute; use the explicit filter either way.
			gain := level
			if mute {
				gain = 0
			}
			filter := "volume=" + strconv.FormatFloat(gain, 'g', -1, 64)

			ffargs := []string{
				"-i", in,
				"-af", filter,
				"-c:v", "copy",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
