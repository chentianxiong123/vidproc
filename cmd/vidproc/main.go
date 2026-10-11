package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/example/vidproc/internal/ffmpeg"
	"github.com/example/vidproc/internal/ops"
)

const version = "0.2.0"

var (
	ffmpegPath  string
	ffprobePath string
	dryRun      bool
	showVersion bool
)

func init() {
	flag.StringVar(&ffmpegPath, "ffmpeg", "", "path to ffmpeg binary")
	flag.StringVar(&ffprobePath, "ffprobe", "", "path to ffprobe binary")
	flag.BoolVar(&dryRun, "dry-run", false, "print commands without executing")
	// Declared here instead of hand-checked after flag.Parse(): an undefined
	// flag makes Parse() print usage and exit before main() gets control.
	flag.BoolVar(&showVersion, "v", false, "print version")
	flag.BoolVar(&showVersion, "version", false, "print version")
}

func usage() {
	fmt.Fprint(os.Stderr, `vidproc `+version+` — MP4 video processing toolkit

Usage: vidproc [global flags] <command> [args]

Global flags:
  --ffmpeg <path>    path to ffmpeg binary (also picks up ffprobe next to it)
  --ffprobe <path>   path to ffprobe binary
  --dry-run          print commands without executing
  -h, --help         show this help
  -v, --version      print version

ffmpeg/ffprobe otherwise come from VIDPROC_FFMPEG / VIDPROC_FFPROBE, then PATH.
Set VIDPROC_FFMPEG to pin a version without editing PATH.

Commands:
`)
	for _, op := range ops.List() {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", op.Name, op.Description)
		fmt.Fprintf(os.Stderr, "  %13s %s\n", "", op.Usage)
		fmt.Fprintf(os.Stderr, "\n")
	}
	fmt.Fprint(os.Stderr, `  run          Run a pipeline or workflow
    vidproc run -c "<pipeline>" -i <input> -o <output> [--force]
    vidproc run -f <workflow.yaml>

Pipeline syntax:
  cut -ss 0:10 -t 5 | scale -preset 720p | nvenc -preset high
  Each step is an operation with its own flags.
  The first step gets the pipeline's -i, the last gets -o.

YAML workflow:
  input: input.mp4
  output: output.mp4
  steps:
    - "cut -ss 0:10 -t 5"
    - "scale -preset 720p"
    - "nvenc -preset high"

Run 'vidproc <command> -h' for command-specific help.
`)
}

func newConfig() (*ffmpeg.Config, error) {
	cfg, err := ffmpeg.Resolve(ffmpegPath, ffprobePath)
	if err != nil {
		return nil, err
	}
	if dryRun {
		os.Setenv("VIDPROC_DRY_RUN", "1")
	}
	return cfg, nil
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	os.Exit(1)
}

// globalFlags are accepted at any position on the command line. Go's flag
// package stops at the first positional argument, so "vidproc run ... --dry-run"
// would otherwise reach the command's own FlagSet and be silently ignored.
// Keys are bare flag names because Go treats "-x" and "--x" as the same flag,
// so leading dashes are stripped before lookup. Values indicate whether the
// flag takes an argument.
var globalFlags = map[string]bool{
	"ffmpeg":  true,
	"ffprobe": true,
	"dry-run": false,
	"v":       false,
	"version": false,
}

// hoistGlobals moves global flags (and their values) ahead of the positional
// arguments so that flag.Parse() sees them regardless of where they were given.
func hoistGlobals(args []string) []string {
	var globals, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		key := a
		if eq := strings.IndexByte(a, '='); eq > 0 {
			key = a[:eq]
		}
		if strings.HasPrefix(key, "-") {
			key = strings.TrimLeft(key, "-")
		}
		takesValue, ok := globalFlags[key]
		if !ok {
			rest = append(rest, a)
			continue
		}
		globals = append(globals, a)
		if takesValue && i+1 < len(args) {
			i++
			globals = append(globals, args[i])
		}
	}
	return append(globals, rest...)
}

func main() {
	flag.Usage = usage
	os.Args = append([]string{os.Args[0]}, hoistGlobals(os.Args[1:])...)
	flag.Parse()

	args := flag.Args()
	// Must precede the no-arguments check: "vidproc -v" leaves flag.Args() empty.
	if showVersion {
		fmt.Println(version)
		return
	}
	if len(args) == 0 {
		usage()
		os.Exit(0)
	}

	cfg, err := newConfig()
	if err != nil {
		fatal(err.Error())
	}

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "run":
		err = cmdRun(cfg, cmdArgs)
	default:
		// Try to find the op
		op, ok := ops.Get(cmd)
		if !ok {
			fatal("unknown command: " + cmd)
		}
		// Create flags and parse
		fs := op.NewFlags()
		// The top-level help promises "vidproc <command> -h" shows that command's
		// flags, but a ContinueOnError FlagSet only reports an undefined flag.
		// Only offered when the op has no -h of its own: crop, scale and title
		// use -h for height.
		if fs.Lookup("h") == nil {
			for _, a := range cmdArgs {
				if a == "-h" || a == "--help" {
					fmt.Fprintln(os.Stderr, op.Usage)
					return
				}
			}
		}
		if err := fs.Parse(cmdArgs); err != nil {
			fatal(err.Error())
		}
		// Go's flag parser stops at the first positional argument, so flags written
		// after one would be silently dropped. Fail loudly instead: this is what
		// broke "mark -i in logo.png -o out" (the -o was never parsed).
		for _, a := range fs.Args() {
			if len(a) > 0 && a[0] == '-' {
				fatal("flags must come before positional arguments, found " + a +
					" (Go stops parsing there, so it would be silently ignored)")
			}
		}
		// Execute with empty in/out (standalone mode)
		err = op.Execute(cfg, "", "", fs)
	}

	if err != nil {
		fatal(err.Error())
	}
}

func cmdRun(cfg *ffmpeg.Config, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	cmd := fs.String("c", "", "pipeline command string")
	file := fs.String("f", "", "workflow YAML file")
	input := fs.String("i", "", "input file (for pipeline)")
	output := fs.String("o", "", "output file (for pipeline)")
	force := fs.Bool("force", false, "overwrite output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *cmd != "" {
		// Pipeline mode
		steps, err := ParsePipeline(*cmd)
		if err != nil {
			return err
		}
		if *input == "" || *output == "" {
			return errors.New("run: -i <input> and -o <output> required for pipeline")
		}
		pipeline := &Pipeline{
			Input:  *input,
			Output: *output,
			Steps:  steps,
			Force:  *force,
		}
		return ExecutePipeline(cfg, pipeline)
	}

	if *file != "" {
		// YAML workflow mode
		wf, err := ParseYAML(*file)
		if err != nil {
			return err
		}
		steps, err := ParsePipeline(wf.ToPipeline())
		if err != nil {
			return err
		}
		pipeline := &Pipeline{
			Input:  wf.Input,
			Output: wf.Output,
			Steps:  steps,
			Force:  *force,
		}
		return ExecutePipeline(cfg, pipeline)
	}

	return errors.New("run: use -c <pipeline> or -f <workflow.yaml>")
}