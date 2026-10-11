package ops

import (
	"errors"
	"flag"
	"math"
	"strconv"
	"strings"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "speed",
		Description: "Change playback speed",
		Usage:       "speed -i <input> -o <output> -rate <n> [-noaudio]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("speed", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.Float64("rate", 0, "speed multiplier: 2 = 2x faster, 0.5 = half speed")
			fs.Bool("noaudio", false, "drop the audio stream")
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
			rate := fs.Lookup("rate").Value.(flag.Getter).Get().(float64)
			if in == "" || out == "" {
				return errors.New("speed: -i <input> -o <output> required")
			}
			if rate <= 0 || math.IsInf(rate, 0) || math.IsNaN(rate) {
				return errors.New("speed: -rate must be a positive number")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			rateStr := strconv.FormatFloat(rate, 'g', -1, 64)
			// setpts divides PTS by the rate, so rate>1 is faster and rate<1 is slower.
			ffargs := []string{
				"-i", in,
				"-vf", "setpts=PTS/" + rateStr,
				"-c:v", "libx264",
			}

			if fs.Lookup("noaudio").Value.String() == "true" {
				ffargs = append(ffargs, "-an")
			} else {
				chain := atempoChain(rate)
				if len(chain) == 0 {
					ffargs = append(ffargs, "-c:a", "aac")
				} else {
					ffargs = append(ffargs, "-af", strings.Join(chain, ","))
					ffargs = append(ffargs, "-c:a", "aac")
				}
			}
			ffargs = append(ffargs, "-y", out)

			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
