package ops

import (
	"flag"

	"github.com/example/vidproc/internal/ffmpeg"
)

// Op is an atomic video operation.
// Each op registers its own flags and executes a single action.
type Op struct {
	Name        string
	Description string
	Usage       string
	// NewFlags creates a fresh FlagSet with all flags for this op.
	NewFlags func() *flag.FlagSet
	// Execute runs the op.
	//   cfg: ffmpeg config
	//   in:  input path (may be empty in standalone mode; use fs.Lookup("i") then)
	//   out: output path (may be empty in standalone mode; use fs.Lookup("o") then)
	//   fs:  parsed flags (already fs.Parse'd)
	Execute func(cfg *ffmpeg.Config, in, out string, fs *flag.FlagSet) error
}

var registry = map[string]*Op{}

// Register adds an op to the global registry.
func Register(op *Op) {
	registry[op.Name] = op
}

// Get looks up an op by name.
func Get(name string) (*Op, bool) {
	op, ok := registry[name]
	return op, ok
}

// Names returns all registered op names, sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	// simple sort
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// List returns all registered ops, sorted by name.
func List() []*Op {
	names := Names()
	out := make([]*Op, 0, len(names))
	for _, n := range names {
		out = append(out, registry[n])
	}
	return out
}