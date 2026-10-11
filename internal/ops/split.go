package ops

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"strconv"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "split",
		Description: "Split video into fixed-length segments",
		Usage:       "split -i <input> -o <prefix> -every <seconds> [-copy]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("split", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file prefix (required; files become <prefix>0.mp4, 1.mp4, ...)")
			fs.String("every", "", "segment length in seconds (required)")
			fs.Bool("copy", false, "lossless stream copy; cuts snap to the nearest keyframe")
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
			every := fs.Lookup("every").Value.String()
			force := fs.Lookup("force").Value.String() == "true"
			copy := fs.Lookup("copy").Value.String() == "true"
			dryRun := DryRunFlag()

			if in == "" || out == "" {
				return errors.New("split: -i <input> -o <prefix> required")
			}
			if every == "" {
				return errors.New("split: -every <seconds> required")
			}
			everySec, err := strconv.ParseFloat(every, 64)
			if err != nil || everySec <= 0 {
				return errors.New("split: -every must be a positive number")
			}
			if err := CheckOutput(force, out+"0.mp4"); err != nil {
				return err
			}

			dur, err := probeDuration(cfg, in)
			if err != nil {
				return fmt.Errorf("split: %w", err)
			}

			n := int(math.Ceil(dur / everySec))
			if n < 1 {
				n = 1
			}
			for i := 0; i < n; i++ {
				start := float64(i) * everySec
				t := everySec
				if start+t > dur {
					t = dur - start
				}
				if t <= 0 {
					break
				}
				// -ss before -i: frame-accurate when re-encoding (ffmpeg decodes
				// from the preceding keyframe and drops frames), and a keyframe
				// seek when copying.
				args := []string{
					"-ss", strconv.FormatFloat(start, 'f', 3, 64),
					"-t", strconv.FormatFloat(t, 'f', 3, 64),
					"-i", in,
				}
				if copy {
					args = append(args, "-c", "copy")
				} else {
					args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-c:a", "aac")
				}
				args = append(args, "-y", out+strconv.Itoa(i)+".mp4")
				if err := RunFFmpeg(cfg, args, dryRun); err != nil {
					return fmt.Errorf("split: segment %d: %w", i, err)
				}
			}
			return nil
		},
	})
}
