package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vidproc/internal/ffmpeg"
	"vidproc/internal/ops"
)

// Pipeline represents a chain of operations.
type Pipeline struct {
	Input  string
	Output string
	Steps  []Step
	Force  bool
}

// Step represents a single operation in a pipeline.
type Step struct {
	OpName string
	Args   []string
}

// ParsePipeline parses a pipeline string like "cut -ss 0:10 | scale -h 720 | nvenc".
func ParsePipeline(s string) ([]Step, error) {
	if s == "" {
		return nil, errors.New("empty pipeline")
	}
	parts := strings.Split(s, "|")
	steps := make([]Step, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		step := Step{
			OpName: fields[0],
			Args:   fields[1:],
		}
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return nil, errors.New("no valid steps in pipeline")
	}
	return steps, nil
}

// Execute runs the pipeline.
func ExecutePipeline(cfg *ffmpeg.Config, pipeline *Pipeline) error {
	if pipeline.Input == "" || pipeline.Output == "" {
		return errors.New("pipeline: input and output required")
	}
	if len(pipeline.Steps) == 0 {
		return errors.New("pipeline: no steps")
	}

	// Check output exists
	if !pipeline.Force {
		if _, err := os.Stat(pipeline.Output); err == nil {
			return fmt.Errorf("output %s exists; use --force to overwrite", pipeline.Output)
		}
	}

	// Create temp directory for intermediate files
	tmpDir, err := os.MkdirTemp("", "vidproc-pipeline-*")
	if err != nil {
		return fmt.Errorf("pipeline: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Execute each step
	var currentIn string
	for i, step := range pipeline.Steps {
		// Determine input and output for this step
		var in, out string
		if i == 0 {
			in = pipeline.Input
		} else {
			in = currentIn
		}
		if i == len(pipeline.Steps)-1 {
			out = pipeline.Output
		} else {
			out = filepath.Join(tmpDir, fmt.Sprintf("step_%d.mp4", i))
		}

		// Get the op
		op, ok := ops.Get(step.OpName)
		if !ok {
			return fmt.Errorf("pipeline: unknown op %s", step.OpName)
		}

		// Create flags and parse args
		fs := op.NewFlags()
		// Add -i and -o to args
		args := step.Args
		args = append(args, "-i", in, "-o", out)
		// Add -force if needed (for the last step, or if temp files might exist)
		if i == len(pipeline.Steps)-1 && pipeline.Force {
			args = append(args, "-force")
		}
		fs.Parse(args)

		// Execute
		if err := op.Execute(cfg, in, out, fs); err != nil {
			return fmt.Errorf("pipeline step %d (%s): %w", i+1, step.OpName, err)
		}

		currentIn = out
		fmt.Fprintf(os.Stderr, "  [%d/%d] %s: %s -> %s\n", i+1, len(pipeline.Steps), step.OpName, in, out)
	}

	return nil
}
