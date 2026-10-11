package ops

import (
	"errors"
	"flag"
	"os"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "text",
		Description: "Overlay text on video",
		Usage:       "text -i <input> -o <output> -text <s> [-x <n>] [-y <n>] [-font <path>] [-size <n>] [-color <c>] [-box] [-border <n>] [-start <t>] [-end <t>]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("text", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("text", "", "text to render (required)")
			fs.String("x", "", "x position, or expression (default: centered)")
			fs.String("y", "", "y position, or expression (default: centered)")
			fs.String("font", "", "font file path (default: auto-detected)")
			fs.String("size", "32", "font size")
			fs.String("color", "white", "text color")
			fs.Bool("box", false, "draw a box behind the text")
			fs.String("boxcolor", "black@0.5", "box color")
			fs.String("boxpad", "10", "box border width")
			fs.String("border", "", "text border width (outline)")
			fs.String("bordercolor", "black", "text border color")
			fs.String("start", "", "show text from this time (seconds)")
			fs.String("end", "", "hide text after this time (seconds)")
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
			text := fs.Lookup("text").Value.String()
			if in == "" || out == "" {
				return errors.New("text: -i <input> -o <output> required")
			}
			if text == "" {
				return errors.New("text: -text <s> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			font := fs.Lookup("font").Value.String()
			if font == "" {
				font = defaultFontFile()
				if font == "" {
					return errors.New("text: no system font found; pass -font <path>")
				}
			}

			textFile, err := writeTempText(text)
			if err != nil {
				return err
			}
			defer os.Remove(textFile)

			start, end := fs.Lookup("start").Value.String(), fs.Lookup("end").Value.String()
			var enable string
			if start != "" || end != "" {
				if start == "" {
					start = "0"
				}
				if end == "" {
					end = "99999"
				}
				enable = "between(t," + start + "," + end + ")"
			}

			ffargs := []string{
				"-i", in,
				"-vf", drawtextFilter(drawtextOpts{
					TextFile:    textFile,
					Font:        font,
					Size:        fs.Lookup("size").Value.String(),
					Color:       fs.Lookup("color").Value.String(),
					X:           fs.Lookup("x").Value.String(),
					Y:           fs.Lookup("y").Value.String(),
					Box:         fs.Lookup("box").Value.String() == "true",
					BoxColor:    fs.Lookup("boxcolor").Value.String(),
					BoxPad:      fs.Lookup("boxpad").Value.String(),
					Border:      fs.Lookup("border").Value.String(),
					BorderColor: fs.Lookup("bordercolor").Value.String(),
					Enable:      enable,
				}),
				"-c:a", "copy",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
