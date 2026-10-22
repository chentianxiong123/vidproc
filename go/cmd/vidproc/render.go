package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"vidproc/internal/cmdrender"
	"vidproc/internal/cli"
)

// cmdRender compiles a declarative project.yaml into a single-pass
// FFmpeg filtergraph and renders it. Thin shell over cmdrender (the
// pure-function gate shared by CLI/Web/AI).
func cmdRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	outputFlag := fs.String("o", "", "Output file path (overrides spec)")
	gpuFlag := fs.String("gpu", "", "Hardware accelerator (nvenc, videotoolbox, qsv, vaapi, amf)")
	logLevelFlag := fs.String("log-level", "info", "Log level (debug, info, warn, error)")
	dryRunFlag := fs.Bool("dry-run", false, "Compile without invoking FFmpeg render")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("render: missing specification file path. Usage: vidproc render <project.yaml> [flags]")
	}

	specPath := fs.Arg(0)
	opts := cmdrender.Options{
		Output: *outputFlag,
		DryRun: *dryRunFlag || dryRun,
		GPU:    *gpuFlag,
	}

	// dry-run: 输出 filtergraph 不回显进度
	if opts.DryRun {
		fg, err := cmdrender.Compile(context.Background(), specPath, opts)
		if err != nil {
			return err
		}
		fmt.Printf("Filtergraph compiled with %d DAG nodes\n", fg.Nodes)
		fmt.Printf("\nGenerated FFmpeg Arguments:\nffmpeg %s\n", strings.Join(fg.Args, " "))
		return nil
	}

	logger := cli.SetupLogger(*logLevelFlag, "text", os.Stderr)
	opts.Logger = logger

	start := time.Now()
	result, err := cmdrender.Render(context.Background(), specPath, opts)
	if err != nil {
		return err
	}

	var sizeStr string
	if result.Size > 0 {
		sizeStr = fmt.Sprintf("%.2f MB", float64(result.Size)/(1<<20))
	}
	fmt.Printf("Render complete: %s (%s, %s)\n", result.Path, sizeStr, time.Since(start).Round(10*time.Millisecond))
	return nil
}