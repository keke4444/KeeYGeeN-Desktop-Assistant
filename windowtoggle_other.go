//go:build !windows

package main

// toggleMainWindow 在非 Windows 平台为空实现（热键同样暂未支持）。
func toggleMainWindow() {}
