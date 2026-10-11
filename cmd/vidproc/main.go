package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/example/vidproc/internal/ffmpeg"
	"github.com/example/vidproc/internal/mp4"
)

const version = "0.1.0"

var (
	ffmpegPath  string
	ffprobePath string
	dryRun      bool
	logLevel    string
)

func init() {
	flag.StringVar(&ffmpegPath, "ffmpeg", "", "path to ffmpeg binary")
	flag.StringVar(&ffprobePath, "ffprobe", "", "path to ffprobe binary")
	flag.BoolVar(&dryRun, "dry-run", false, "print commands without executing")
	flag.StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
}

func usage() {
	fmt.Fprint(os.Stderr, `vidproc `+version+` — MP4 metadata, concat & editing tool

Usage: vidproc [global flags] <command> [args]

Global flags:
  --ffmpeg <path>    path to ffmpeg binary
  --ffprobe <path>   path to ffprobe binary
  --dry-run          print commands without executing
  --log-level <lvl>  debug|info|warn|error
  -h, --help         show this help
  -v, --version      print version

Commands:
  probe <files...>                  extract metadata as JSON
  concat <files...> -o <out>        lossless concatenation
  cut <input> -o <out> -ss <t> -t <t>  fast/accurate cut
  transcode <input> -o <out> [opts]   re-encode with options (GPU/CPU)
  metadata <input> -o <out> [flags]   read/write format tags
  boxes <list|extract|inject> ...     raw MP4 box operations
  timestamp <target> <ref>          copy mtime

Run 'vidproc <command> -h' for command-specific help.
`)
}

func newConfig() (*ffmpeg.Config, error) {
	cfg, err := ffmpeg.NewConfig()
	if err != nil {
		return nil, err
	}
	if ffmpegPath != "" {
		cfg = cfg.WithFFmpeg(ffmpegPath)
	}
	if ffprobePath != "" {
		cfg = cfg.WithFFprobe(ffprobePath)
	}
	return cfg, nil
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() == 0 {
		if dryRun {
			fmt.Println("[dry-run] no command given")
			return
		}
		usage()
		fatal("no command given")
	}

	cmd := flag.Arg(0)
	args := flag.Args()[1:]

	cfg, err := newConfig()
	if err != nil {
		fatal(err.Error())
	}

	switch cmd {
	case "probe":
		err = cmdProbe(cfg, args)
	case "concat":
		err = cmdConcat(cfg, args)
	case "cut":
		err = cmdCut(cfg, args)
	case "transcode":
		err = cmdTranscode(cfg, args)
	case "metadata":
		err = cmdMetadata(cfg, args)
	case "boxes":
		err = cmdBoxes(cfg, args)
	case "timestamp":
		err = cmdTimestamp(cfg, args)
	default:
		fatal("unknown command: " + cmd)
	}
	if err != nil {
		fatal(err.Error())
	}
}

// --- Commands ---

func cmdProbe(cfg *ffmpeg.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("probe: at least one file required")
	}
	for _, f := range args {
		res, err := cfg.Probe(f)
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
	}
	return nil
}

func cmdConcat(cfg *ffmpeg.Config, args []string) error {
	fs := flag.NewFlagSet("concat", flag.ContinueOnError)
	output := fs.String("o", "", "output file (required)")
	force := fs.Bool("force", false, "overwrite output")
	fs.Parse(args)

	if *output == "" {
		return errors.New("concat: -o <output> required")
	}
	inputs := fs.Args()
	if len(inputs) < 2 {
		return errors.New("concat: need at least 2 input files")
	}

	if !*force {
		if _, err := os.Stat(*output); err == nil {
			return fmt.Errorf("output %s exists; use --force to overwrite", *output)
		}
	}

	return cfg.Concat(inputs, *output)
}

func cmdCut(cfg *ffmpeg.Config, args []string) error {
	fs := flag.NewFlagSet("cut", flag.ContinueOnError)
	input := fs.String("i", "", "input file (required)")
	output := fs.String("o", "", "output file (required)")
	ss := fs.String("ss", "", "start time (e.g. 00:00:10)")
	duration := fs.String("t", "", "duration (e.g. 00:00:30)")
	accurate := fs.Bool("accurate", false, "accurate cut (re-encode) instead of fast seek")
	force := fs.Bool("force", false, "overwrite output")
	fs.Parse(args)

	if *input == "" || *output == "" {
		return errors.New("cut: -i <input> -o <output> required")
	}
	if *ss == "" && *duration == "" {
		return errors.New("cut: at least one of -ss or -t required")
	}
	if !*force {
		if _, err := os.Stat(*output); err == nil {
			return fmt.Errorf("output %s exists; use --force to overwrite", *output)
		}
	}

	ffargs := []string{}
	if *ss != "" {
		ffargs = append(ffargs, "-ss", *ss)
	}
	ffargs = append(ffargs, "-i", *input)
	if *duration != "" {
		ffargs = append(ffargs, "-t", *duration)
	}
	if *accurate {
		ffargs = append(ffargs, "-c:v", "libx264", "-c:a", "aac")
	} else {
		ffargs = append(ffargs, "-c", "copy")
	}
	ffargs = append(ffargs, "-y", *output)

	if dryRun {
		fmt.Fprintf(os.Stderr, "[dry-run] %s %s\n", cfg.FFmpeg, strings.Join(ffargs, " "))
		return nil
	}
	cmd := exec.Command(cfg.FFmpeg, ffargs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cmdTranscode(cfg *ffmpeg.Config, args []string) error {
	fs := flag.NewFlagSet("transcode", flag.ContinueOnError)
	input := fs.String("i", "", "input file (required)")
	output := fs.String("o", "", "output file (required)")
	vcodec := fs.String("c:v", "h264", "video codec: h264, hevc, nvenc_hw (GPU)")
	acodec := fs.String("c:a", "aac", "audio codec: aac, mp3, ac3")
	crf := fs.Int("crf", 23, "CRF quality (0-51, lower=better)")
	preset := fs.String("preset", "medium", "x264/x265 encoding preset")
	list_presets := fs.Bool("list-presets", false, "list available presets and exit")
	pixfmt := fs.String("pix_fmt", "yuv420p", "pixel format: yuv420p, yuv444p")
	tune := fs.String("tune", "zerolatency", "tune objective: zerolatency, ssim, psnr, fastdecode")
	encode_preset := fs.String("enc-preset", "medium", "NVENC encoding preset: medium, hq, hp, p4, p5, ld, fast")
	force := fs.Bool("force", false, "overwrite output")
	fs.Parse(args)

	if *input == "" || *output == "" {
		return errors.New("transcode: -i <input> -o <output> required")
	}
	if !*force {
		if _, err := os.Stat(*output); err == nil {
			return fmt.Errorf("output %s exists; use --force to overwrite", *output)
		}
	}

	if *list_presets {
		fmt.Fprintf(os.Stderr, "Available encoding presets:\n")
		fmt.Fprintf(os.Stderr, "  Video codecs:\n")
		fmt.Fprintf(os.Stderr, "    h264        - CPU x264 (default)\n")
		fmt.Fprintf(os.Stderr, "    hevc        - CPU x265\n")
		fmt.Fprintf(os.Stderr, "    nvenc_hw    - NVIDIA NVENC GPU (requires CUDA)\n")
		fmt.Fprintf(os.Stderr, "\n  x264/x265 presets: ultrafast, superfast, veryfast, faster, fast, medium, slow, slower, veryslow\n")
		fmt.Fprintf(os.Stderr, "\n  NVENC presets (--enc-preset):\n")
		fmt.Fprintf(os.Stderr, "    p4          - Highest quality (slowest)\n")
		fmt.Fprintf(os.Stderr, "    p5          - High quality\n")
		fmt.Fprintf(os.Stderr, "    medium      - Balanced (default)\n")
		fmt.Fprintf(os.Stderr, "    hq          - High quality\n")
		fmt.Fprintf(os.Stderr, "    hp          - High performance\n")
		fmt.Fprintf(os.Stderr, "    ld          - Low latency\n")
		fmt.Fprintf(os.Stderr, "    fast        - Fast\n")
		fmt.Fprintf(os.Stderr, "\n  Tune objectives: zerolatency, ssim, psnr, fastdecode\n")
		os.Exit(0)
	}

	ffargs := []string{"-i", *input}

	switch *vcodec {
	case "h264":
		ffargs = append(ffargs, "-c:v", "libx264")
		ffargs = append(ffargs, "-crf", fmt.Sprintf("%d", *crf))
		ffargs = append(ffargs, "-preset", *preset)
		if *tune != "" {
			ffargs = append(ffargs, "-tune", *tune)
		}
	case "hevc":
		ffargs = append(ffargs, "-c:v", "libx265")
		ffargs = append(ffargs, "-crf", fmt.Sprintf("%d", *crf))
		ffargs = append(ffargs, "-preset", *preset)
		if *tune != "" {
			ffargs = append(ffargs, "-tune", *tune)
		}
	case "nvenc_hw":
		ffargs = append(ffargs, "-c:v", "h264_nvenc")
		ffargs = append(ffargs, "-preset", *encode_preset)
		ffargs = append(ffargs, "-crf", fmt.Sprintf("%d", *crf))
		ffargs = append(ffargs, "-tune", *tune)
		ffargs = append(ffargs, "-pix_fmt", *pixfmt)
	default:
		ffargs = append(ffargs, "-c:v", "libx264")
		ffargs = append(ffargs, "-crf", fmt.Sprintf("%d", *crf))
		ffargs = append(ffargs, "-preset", *preset)
	}

	ffargs = append(ffargs, "-c:a", *acodec)
	ffargs = append(ffargs, "-pix_fmt", *pixfmt)
	ffargs = append(ffargs, "-y", *output)

	if dryRun {
		fmt.Fprintf(os.Stderr, "[dry-run] %s %s\n", cfg.FFmpeg, strings.Join(ffargs, " "))
		return nil
	}
	cmd := exec.Command(cfg.FFmpeg, ffargs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cmdMetadata(cfg *ffmpeg.Config, args []string) error {
	fs := flag.NewFlagSet("metadata", flag.ContinueOnError)
	input := fs.String("i", "", "input file (required)")
	output := fs.String("o", "", "output file (required)")
	copyFrom := fs.String("copy-from", "", "copy ALL format tags from source file")
	setTags := stringSliceFlag{}
	delTags := stringSliceFlag{}
	stripAll := fs.Bool("strip-all", false, "remove all format tags before applying")
	fs.Var(&setTags, "set", "set tag key=value (repeatable)")
	fs.Var(&delTags, "delete", "delete tag key (repeatable)")
	fs.Parse(args)

	if *input == "" || *output == "" {
		return errors.New("metadata: -i <input> -o <output> required")
	}

	ffargs := []string{"-i", *input}

	if *copyFrom != "" {
		ffargs = append(ffargs, "-i", *copyFrom, "-map_metadata", "1")
	} else if *stripAll {
		ffargs = append(ffargs, "-map_metadata", "-1")
	}

	for _, kv := range setTags {
		ffargs = append(ffargs, "-metadata", kv)
	}
	for _, k := range delTags {
		ffargs = append(ffargs, "-metadata", k+"=")
	}

	ffargs = append(ffargs, "-c", "copy", "-y", *output)

	if dryRun {
		fmt.Fprintf(os.Stderr, "[dry-run] %s %s\n", cfg.FFmpeg, strings.Join(ffargs, " "))
		return nil
	}
	cmd := exec.Command(cfg.FFmpeg, ffargs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error { *s = append(*s, v); return nil }

func cmdBoxes(cfg *ffmpeg.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("boxes: subcommand required: list|extract|inject")
	}
	sub := args[0]
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
		fs := flag.NewFlagSet("boxes extract", flag.ContinueOnError)
		match := fs.String("match", "", "substring match on payload")
		dedup := fs.Bool("dedup", false, "dedup identical payloads")
		fs.Parse(args[2:])
		if len(fs.Args()) < 2 {
			return errors.New("boxes extract: need <box-type> <file>")
		}
		boxType := fs.Args()[0]
		file := fs.Args()[1]
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		boxes := mp4.FilterBoxes(mp4.ScanBoxes(data), boxType, *match, *dedup)
		for _, b := range boxes {
			os.Stdout.Write(b.Payload)
		}
	case "inject":
		fs := flag.NewFlagSet("boxes inject", flag.ContinueOnError)
		boxType := fs.String("box-type", "", "only inject this box type (default: all non-standard)")
		match := fs.String("match", "", "substring match on payload")
		dedup := fs.Bool("dedup", true, "dedup identical payloads")
		fs.Parse(args[2:])
		if len(fs.Args()) < 2 {
			return errors.New("boxes inject: need <dst> <src>")
		}
		dst, src := fs.Args()[0], fs.Args()[1]
		srcData, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		srcBoxes := mp4.ScanBoxes(srcData)
		var toInject []mp4.Box
		if *boxType != "" {
			toInject = mp4.FilterBoxes(srcBoxes, *boxType, *match, *dedup)
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
}

func cmdTimestamp(cfg *ffmpeg.Config, args []string) error {
	if len(args) != 2 {
		return errors.New("timestamp: usage: vidproc timestamp <target> <reference>")
	}
	target, ref := args[0], args[1]
	refInfo, err := os.Stat(ref)
	if err != nil {
		return err
	}
	return os.Chtimes(target, refInfo.ModTime(), refInfo.ModTime())
}

