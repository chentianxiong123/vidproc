package ops

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"vidproc/internal/ffmpeg"
)

// 从文件名识别录制时间，支持两种常见命名：
//   VID_20250223_104321.mp4                (安卓/相机)
//   Screenrecorder-2025-05-03-20-09-58.mp4 (录屏工具)
var (
	vidDateRe     = regexp.MustCompile(`VID_(\d{8})_(\d{6})`)
	recorderDateRe = regexp.MustCompile(`Screenrecorder-(\d{4})-(\d{2})-(\d{2})-(\d{2})-(\d{2})-(\d{2})`)
)

// fileDateTime 从文件名提取 (YYYYMMDD, HHMMSS)；识别不了就回退到文件 mtime。
func fileDateTime(path string) (string, string, error) {
	base := filepath.Base(path)
	if m := vidDateRe.FindStringSubmatch(base); m != nil {
		return m[1], m[2], nil
	}
	if m := recorderDateRe.FindStringSubmatch(base); m != nil {
		return m[1] + m[2] + m[3], m[4] + m[5] + m[6], nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", "", err
	}
	t := fi.ModTime()
	return t.Format("20060102"), t.Format("150405"), nil
}

// autoConcatName 生成输出文件名，模仿实际目录里的命名习惯：
//   同一天:  VID_20250223_104321_to_140414.mp4
//   跨天:    VID_20250209_180531_to_20250209_192457.mp4
func autoConcatName(first, last string) (string, error) {
	d1, t1, err := fileDateTime(first)
	if err != nil {
		return "", err
	}
	d2, t2, err := fileDateTime(last)
	if err != nil {
		return "", err
	}
	if d1 == d2 {
		return fmt.Sprintf("VID_%s_%s_to_%s.mp4", d1, t1, t2), nil
	}
	return fmt.Sprintf("VID_%s_%s_to_%s_%s.mp4", d1, t1, d2, t2), nil
}

func init() {
	Register(&Op{
		Name:        "concat",
		Description: "Concatenate multiple videos (lossless)",
		Usage:       "concat [-auto-name] [-o <output>] [--force] [--keep-mtime] <files...>",
		NewFlags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("concat", flag.ContinueOnError)
			fs.String("o", "", "output file (optional when -auto-name is set)")
			fs.Bool("auto-name", false, "auto-generate output name from first/last input timestamps")
			fs.Bool("force", false, "overwrite output")
			fs.Bool("keep-mtime", false, "keep mtime from first input file")
			return fs
		},
		Execute: func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error {
			if out == "" {
				out = fs.Lookup("o").Value.String()
			}
			inputs := fs.Args()
			for _, a := range inputs {
				if len(a) > 0 && a[0] == '-' {
					return errors.New("concat: flags must come before the file list, e.g. concat -o out.mp4 [--force] a.mp4 b.mp4")
				}
			}
			if len(inputs) < 2 {
				return errors.New("concat: need at least 2 input files")
			}
			autoName := fs.Lookup("auto-name").Value.String() == "true"
			if out == "" && !autoName {
				return errors.New("concat: -o <output> or -auto-name required")
			}
			if out == "" && autoName {
				name, err := autoConcatName(inputs[0], inputs[len(inputs)-1])
				if err != nil {
					return fmt.Errorf("concat: -auto-name: %w", err)
				}
				out = name
				fmt.Fprintf(os.Stderr, "concat: auto output name: %s\n", out)
			}
			if err := CheckOutput(fs.Lookup("force").Value.String() == "true", out); err != nil {
				return err
			}
			if err := cfg.Concat(inputs, out); err != nil {
				return err
			}
			if fs.Lookup("keep-mtime").Value.String() == "true" && len(inputs) > 0 {
				fi, err := os.Stat(inputs[0])
				if err != nil {
					return err
				}
				_ = os.Chtimes(out, fi.ModTime(), fi.ModTime())
			}
			return nil
		},
	})
}