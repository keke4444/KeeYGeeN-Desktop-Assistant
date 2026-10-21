//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// DesktopDir 解析当前用户的桌面目录。
// 主方案：读注册表 User Shell Folders，避免中文系统「桌面 / Desktop」显示名差异。
func DesktopDir() (string, error) {
	if p, err := desktopFromRegistry(); err == nil && p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法定位用户主目录: %w", err)
	}
	candidates := []string{
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "OneDrive", "Desktop"),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("未找到桌面目录，请在配置文件中手动指定 desktop")
}

func desktopFromRegistry() (string, error) {
	const key = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`
	cmd := exec.Command("reg", "query", key, "/v", "Desktop")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		idx := strings.Index(line, "REG_")
		if idx < 0 {
			continue
		}
		rest := line[idx:]
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(rest, fields[0]))
		if val == "" {
			continue
		}
		val = os.ExpandEnv(val)
		if fi, err := os.Stat(val); err == nil && fi.IsDir() {
			return val, nil
		}
	}
	return "", fmt.Errorf("注册表 Desktop 项无效")
}
