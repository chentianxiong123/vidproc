package ops

import (
	"errors"
	"flag"

	"vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "cut",
		Description: "Trim video by time range",
		Usage:       "cut -i <input> -o <output> [-ss <t>] [-t <t>] [-accurate]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("cut", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("ss", "", "start time (e.g. 00:00:10)")
			fs.String("t", "", "duration (e.g. 00:00:30)")
			fs.Bool("accurate", false, "accurate cut (re-encode) instead of fast seek")
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
				return errors.New("cut: -i <input> -o <output> required")
			}
			ss := fs.Lookup("ss").Value.String()
			duration := fs.Lookup("t").Value.String()
			if ss == "" && duration == "" {
				return errors.New("cut: at least one of -ss or -t required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			ffargs := []string{}
			if ss != "" {
				ffargs = append(ffargs, "-ss", ss)
			}
			ffargs = append(ffargs, "-i", in)
			if duration != "" {
				ffargs = append(ffargs, "-t", duration)
			}
			if fs.Lookup("accurate").Value.String() == "true" {
				ffargs = append(ffargs, "-c:v", "libx264", "-c:a", "aac")
			} else {
				ffargs = append(ffargs, "-c", "copy")
			}
			ffargs = append(ffargs, "-y", out)

			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
