package ops

import (
	"errors"
	"flag"
	"os"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "ts",
		Description: "Copy modification time from one file to another",
		Usage:       "ts <target> <reference>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("ts", flag.ContinueOnError)
			return fs
		},
		Execute: func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error {
			args := fs.Args()
			if len(args) != 2 {
				return errors.New("ts: usage: ts <target> <reference>")
			}
			target, ref := args[0], args[1]
			refInfo, err := os.Stat(ref)
			if err != nil {
				return err
			}
			return os.Chtimes(target, refInfo.ModTime(), refInfo.ModTime())
		},
	})
}