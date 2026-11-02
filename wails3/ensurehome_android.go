//go:build android

package main

import (
	"os"
	"path/filepath"
)

// ensureHome Android:应用私有目录,无需处理。
func ensureHome() {}

// vidprocDataPath Android:应用数据目录(内部存储 .config/vidproc)。
func vidprocDataPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vidproc")
}
