package ops

import (
	"errors"
	"flag"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "audio",
		Usage:       "audio -i <input> -o <output> [-codec copy|aac|mp3|opus|flac|pcm_s16le]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("audio", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("codec", "copy", "audio codec, or copy to keep the original")
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
			codec := fs.Lookup("codec").Value.String()
			if in == "" || out == "" {
				return errors.New("audio: -i <input> -o <output> required")
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}

			// -vn drops the video stream; -sn/-dn drop subtitles and data.
			ffargs := []string{
				"-i", in,
				"-vn", "-sn", "-dn",
				"-c:a", codec,
				"-y", out,
			}
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
