//go:build !windows

package main

// startTray / stopTray 在非 Windows 平台为空实现（macOS 可在后续版本接入菜单栏图标）。
func startTray() {}

func stopTray() {}
