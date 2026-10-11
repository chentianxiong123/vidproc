package ops

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "title",
		Description: "Generate a title card",
		Usage:       "title -o <output> -text <s> [-duration <t>] [-w <n>] [-h <n>] [-bg <color>] [-color <color>] [-size <n>] [-font <path>]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("title", flag.ContinueOnError)
			fs.String("i", "", "ignored (a title card is generated, not edited)")
			fs.String("o", "", "output file (required)")
			fs.String("text", "", "title text (required)")
			fs.String("duration", "3", "duration in seconds")
			fs.String("w", "1280", "width in pixels")
			fs.String("h", "720", "height in pixels")
			fs.String("bg", "black", "background color")
			fs.String("color", "white", "text color")
			fs.String("size", "72", "font size")
			fs.String("font", "", "font file path (default: auto-detected)")
			fs.Bool("force", false, "overwrite output")
			return fs
		},
		Execute: func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error {
			if out == "" {
				out = fs.Lookup("o").Value.String()
			}
			text := fs.Lookup("text").Value.String()
			if out == "" {
				return errors.New("title: -o <output> required")
			}
			if text == "" {
				return errors.New("title: -text <s> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			font := fs.Lookup("font").Value.String()
			if font == "" {
				font = defaultFontFile()
				if font == "" {
					return errors.New("title: no system font found; pass -font <path>")
				}
			}

			durStr := fs.Lookup("duration").Value.String()
			dur, err := strconv.ParseFloat(durStr, 64)
			if err != nil || dur <= 0 {
				return errors.New("title: -duration must be a positive number")
			}
			w, err := strconv.Atoi(fs.Lookup("w").Value.String())
			if err != nil || w < 1 {
				return errors.New("title: -w must be a positive integer")
			}
			h, err := strconv.Atoi(fs.Lookup("h").Value.String())
			if err != nil || h < 1 {
				return errors.New("title: -h must be a positive integer")
			}

			textFile, err := writeTempText(text)
			if err != nil {
				return err
			}
			defer os.Remove(textFile)

			bg := fs.Lookup("bg").Value.String()
			size := fs.Lookup("size").Value.String()

			ffargs := []string{
				"-f", "lavfi", "-i", fmt.Sprintf("color=c=%s:s=%dx%d:r=30", bg, w, h),
				"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100",
				"-vf", drawtextFilter(drawtextOpts{
					TextFile: textFile,
					Font:     font,
					Size:     size,
					Color:    fs.Lookup("color").Value.String(),
				}),
				"-t", durStr,
				"-c:v", "libx264", "-pix_fmt", "yuv420p",
				"-c:a", "aac",
				"-shortest",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
