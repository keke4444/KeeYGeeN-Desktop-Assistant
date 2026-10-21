//go:build !windows

package platform

import (
	"os"
	"path/filepath"
)

// DesktopDir 在非 Windows 平台的实现（本轮不作为交付目标，保留以维持可编译）。
func DesktopDir() (string, error) {
	if p := os.Getenv("XDG_DESKTOP_DIR"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Desktop"), nil
}
