package ops

import (
	"errors"
	"flag"
	"strconv"

	"vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "replace-audio",
		Description: "Replace the audio track",
		Usage:       "replace-audio -i <video> -o <output> -a <audio> [-start <t>]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("replace-audio", flag.ContinueOnError)
			fs.String("i", "", "input video (required)")
			fs.String("o", "", "output file (required)")
			fs.String("a", "", "replacement audio file (required)")
			fs.String("start", "", "start the new audio at this time (seconds)")
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
			audio := fs.Lookup("a").Value.String()
			start := fs.Lookup("start").Value.String()
			if in == "" || out == "" || audio == "" {
				return errors.New("replace-audio: -i <video> -o <output> -a <audio> required")
			}
			if start != "" {
				if t, err := strconv.ParseFloat(start, 64); err != nil || t < 0 {
					return errors.New("replace-audio: -start must be a non-negative number")
				}
			}
			force := fs.Lookup("force").Value.String() == "true"
			if err := CheckOutput(force, out); err != nil {
				return err
			}
			if !force && (out == in || out == audio) {
				return errors.New("replace-audio: output would overwrite an input; use --force")
			}

			// Video passes through untouched; the original audio is dropped.
			// -itsoffset shifts the new audio's timestamps forward so it begins
			// at the given time instead of at 0.
			ffargs := []string{"-i", in}
			if start != "" {
				ffargs = append(ffargs, "-itsoffset", start)
			}
			ffargs = append(ffargs,
				"-i", audio,
				"-map", "0:v:0",
				"-c:v", "copy",
				"-map", "1:a:0",
				"-c:a", "aac",
				"-map_metadata", "0",
				"-y", out,
			)
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
