package ops

import (
	"errors"
	"flag"
	"encoding/json"
	"fmt"

	"github.com/example/vidproc/internal/ffmpeg"
)

func init() {
	Register(&Op{
		Name:        "probe",
		Description: "Extract video metadata as JSON",
		Usage:       "probe <file>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("probe", flag.ContinueOnError)
			return fs
		},
		Execute: func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error {
			file := fs.Args()
			if len(file) == 0 {
				return errors.New("probe: file required")
			}
			res, err := cfg.Probe(file[0])
			if err != nil {
				return err
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	})
}