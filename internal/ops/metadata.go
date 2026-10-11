package ops

import (
	"errors"
	"flag"
	"strings"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "metadata",
		Description: "Read/write format tags",
		Usage:       "metadata -i <input> -o <output> [-set k=v;k=v] [-delete k1,k2] [-copy-from <file>] [-strip-all]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("metadata", flag.ContinueOnError)
			fs.String("i", "", "input file (required)")
			fs.String("o", "", "output file (required)")
			fs.String("copy-from", "", "copy ALL format tags from source file")
			fs.String("set", "", "set tags as key=value pairs separated by semicolons (e.g. 'title=My Video;artist=John')")
			fs.String("delete", "", "delete tag keys separated by commas (e.g. 'title,artist')")
			fs.Bool("strip-all", false, "remove all format tags before applying")
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
				return errors.New("metadata: -i <input> -o <output> required")
			}
			copyFrom := fs.Lookup("copy-from").Value.String()
			setStr := fs.Lookup("set").Value.String()
			delStr := fs.Lookup("delete").Value.String()
			stripAll := fs.Lookup("strip-all").Value.String() == "true"
			force := fs.Lookup("force").Value.String() == "true"

			if err := CheckOutput(force, out); err != nil {
				return err
			}

			ffargs := []string{"-i", in}
			if copyFrom != "" {
				ffargs = append(ffargs, "-i", copyFrom, "-map_metadata", "1")
			} else if stripAll {
				ffargs = append(ffargs, "-map_metadata", "-1")
			}

			if setStr != "" {
				for _, kv := range strings.Split(setStr, ";") {
					if kv != "" {
						ffargs = append(ffargs, "-metadata", kv)
					}
				}
			}
			if delStr != "" {
				for _, k := range strings.Split(delStr, ",") {
					if k != "" {
						ffargs = append(ffargs, "-metadata", k+"=")
					}
				}
			}

			ffargs = append(ffargs, "-c", "copy", "-y", out)
			return RunFFmpeg(cfg, ffargs, DryRunFlag())
		},
	})
}
