package ops

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/example/vidproc/internal/ffmpeg"
	"github.com/example/vidproc/internal/mp4"
)

func init() {
	Register(&Op{
		Name:        "boxes",
		Description: "MP4 box operations (list, extract, inject)",
		Usage:       "boxes <list|extract|inject> [args...]",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("boxes", flag.ContinueOnError)
			fs.String("match", "", "substring match on payload (extract/inject)")
			fs.Bool("dedup", false, "dedup identical payloads (extract/inject)")
			fs.String("box-type", "", "only inject this box type (inject)")
			return fs
		},
		Execute: func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error {
			args := fs.Args()
			if len(args) == 0 {
				return errors.New("boxes: subcommand required: list|extract|inject")
			}
			sub := args[0]
			match := fs.Lookup("match").Value.String()
			dedup := fs.Lookup("dedup").Value.String() == "true"
			boxType := fs.Lookup("box-type").Value.String()

			switch sub {
			case "list":
				if len(args) < 2 {
					return errors.New("boxes list: need a file")
				}
				data, err := os.ReadFile(args[1])
				if err != nil {
					return err
				}
				boxes := mp4.ScanBoxes(data)
				for _, b := range boxes {
					std := "non-standard"
					if mp4.StandardBoxes[b.Type] {
						std = "standard"
					}
					fmt.Printf("%-8s  offset=%d  size=%-10d  %s\n", b.Type, b.Offset, b.Size, std)
				}
			case "extract":
				if len(args) < 3 {
					return errors.New("boxes extract: need <box-type> <file>")
				}
				file := args[2]
				data, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				boxes := mp4.FilterBoxes(mp4.ScanBoxes(data), args[1], match, dedup)
				for _, b := range boxes {
					os.Stdout.Write(b.Payload)
				}
			case "inject":
				if len(args) < 3 {
					return errors.New("boxes inject: need <dst> <src>")
				}
				dst, src := args[1], args[2]
				srcData, err := os.ReadFile(src)
				if err != nil {
					return err
				}
				srcBoxes := mp4.ScanBoxes(srcData)
				var toInject []mp4.Box
				if boxType != "" {
					toInject = mp4.FilterBoxes(srcBoxes, boxType, match, dedup)
				} else {
					toInject = mp4.NonStandardBoxes(srcBoxes)
				}
				if err := mp4.AppendBoxes(dst, toInject); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "injected %d boxes into %s\n", len(toInject), dst)
			default:
				return fmt.Errorf("boxes: unknown subcommand %s", sub)
			}
			return nil
		},
	})
}