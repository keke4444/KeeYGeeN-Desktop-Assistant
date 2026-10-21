//go:build !windows

// Package shellicon 在非 Windows 平台不提供系统图标，返回空字符串由前端回退。
package shellicon

// IconFor 返回空字符串（非 Windows 平台）。
func IconFor(path string, isDir bool) string { return "" }

// FolderIcon 返回空字符串（非 Windows 平台）。
func FolderIcon() string { return "" }
