package ops

import (
	"errors"
	"flag"
	"strconv"

	"vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "gif",
		Description: "Convert video to GIF",
		Usage:       "gif -i <input> -o <out.gif> [-ss <t>] [-t <seconds>] [-w <width>] [-fps <n>]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("gif", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output GIF (required)")
			fs.String("ss", "", "start at this time (seconds)")
			fs.String("t", "", "clip duration in seconds (default: to end)")
			fs.String("w", "480", "output width; height scales proportionally")
			fs.String("fps", "12", "frame rate")
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
			if in == "" || out == "" {
				return errors.New("gif: -i <input> -o <out.gif> required")
			}
			w := fs.Lookup("w").Value.String()
			fps := fs.Lookup("fps").Value.String()
			if n, err := strconv.Atoi(w); err != nil || n < 1 {
				return errors.New("gif: -w must be a positive integer")
			}
			if n, err := strconv.ParseFloat(fps, 64); err != nil || n <= 0 {
				return errors.New("gif: -fps must be a positive number")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			// Two-pass palette: generate a palette from one copy of the stream,
			// then use it on the other so the colors stay stable.
			graph := "[0:v]fps=fps=" + fps + ",scale=" + w + ":-1:flags=lanczos,split[a][b];" +
				"[a]palettegen[p];" +
				"[b][p]paletteuse"

			ffargs := []string{}
			if ss := fs.Lookup("ss").Value.String(); ss != "" {
				ffargs = append(ffargs, "-ss", ss)
			}
			ffargs = append(ffargs, "-i", in)
			if t := fs.Lookup("t").Value.String(); t != "" {
				ffargs = append(ffargs, "-t", t)
			}
			ffargs = append(ffargs, "-filter_complex", graph, "-y", out)
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
