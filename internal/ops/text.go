package ops

import (
	"errors"
	"flag"
	"os"
	"strings"

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

			// The text goes through a temp file so arbitrary content
			// (%, :, ', newline, non-ASCII) needs no escaping at all.
			// expansion=none keeps drawtext from interpreting %{...} sequences.
			tf, err := os.CreateTemp("", "vidproc-drawtext-*.txt")
			if err != nil {
				return err
			}
			tmpName := tf.Name()
			defer os.Remove(tmpName)
			if _, err := tf.WriteString(text); err != nil {
				tf.Close()
				return err
			}
			if err := tf.Close(); err != nil {
				return err
			}

			opts := []string{
				"fontfile='" + escapeFilterPath(font) + "'",
				"textfile='" + escapeFilterPath(tmpName) + "'",
				"expansion=none",
				"fontsize='" + fs.Lookup("size").Value.String() + "'",
				"fontcolor='" + fs.Lookup("color").Value.String() + "'",
			}

			xf, yf := fs.Lookup("x").Value.String(), fs.Lookup("y").Value.String()
			if xf == "" {
				xf = "(w-text_w)/2"
			}
			if yf == "" {
				yf = "(h-text_h)/2"
			}
			opts = append(opts, "x='"+xf+"'", "y='"+yf+"'")

			if fs.Lookup("box").Value.String() == "true" {
				opts = append(opts, "box=1",
					"boxcolor='"+fs.Lookup("boxcolor").Value.String()+"'",
					"boxborderw='"+fs.Lookup("boxpad").Value.String()+"'")
			}
			if b := fs.Lookup("border").Value.String(); b != "" {
				opts = append(opts, "borderw='"+b+"'",
					"bordercolor='"+fs.Lookup("bordercolor").Value.String()+"'")
			}
			start, end := fs.Lookup("start").Value.String(), fs.Lookup("end").Value.String()
			if start != "" || end != "" {
				if start == "" {
					start = "0"
				}
				if end == "" {
					end = "99999"
				}
				opts = append(opts, "enable='between(t,"+start+","+end+")'")
			}

			ffargs := []string{
				"-i", in,
				"-vf", "drawtext=" + strings.Join(opts, ":"),
				"-c:a", "copy",
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
