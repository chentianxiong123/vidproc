package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Workflow represents a YAML workflow configuration.
type Workflow struct {
	Input  string
	Output string
	Steps  []string
}

// ParseYAML parses a simple YAML workflow file.
// This is a minimal parser that supports the workflow format:
//
//	input: input.mp4
//	output: output.mp4
//	steps:
//	  - "cut -ss 0:10 -t 5"
//	  - "scale -preset 720p"
func ParseYAML(path string) (*Workflow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	wf := &Workflow{}
	scanner := bufio.NewScanner(f)
	inSteps := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "input:") {
			wf.Input = strings.TrimSpace(strings.TrimPrefix(line, "input:"))
			inSteps = false
		} else if strings.HasPrefix(line, "output:") {
			wf.Output = strings.TrimSpace(strings.TrimPrefix(line, "output:"))
			inSteps = false
		} else if strings.HasPrefix(line, "steps:") {
			inSteps = true
		} else if inSteps && strings.HasPrefix(line, "- ") {
			step := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			// Remove quotes if present
			if (strings.HasPrefix(step, "\"") && strings.HasSuffix(step, "\"")) ||
				(strings.HasPrefix(step, "'") && strings.HasSuffix(step, "'")) {
				step = step[1 : len(step)-1]
			}
			wf.Steps = append(wf.Steps, step)
		} else {
			inSteps = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if wf.Input == "" || wf.Output == "" || len(wf.Steps) == 0 {
		return nil, fmt.Errorf("workflow: input, output, and steps are required")
	}

	return wf, nil
}

// ToPipeline converts a workflow to a pipeline command string.
func (wf *Workflow) ToPipeline() string {
	return strings.Join(wf.Steps, " | ")
}
