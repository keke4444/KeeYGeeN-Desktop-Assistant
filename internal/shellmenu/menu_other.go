//go:build !windows

// Package shellmenu 在非 Windows 平台不提供系统原生右键菜单，
// 前端会回退到内置菜单。
package shellmenu

import "errors"

var errUnsupported = errors.New("系统原生右键菜单仅在 Windows 下可用")

// Show 在非 Windows 平台直接返回错误。
func Show(paths []string) error { return errUnsupported }

// ShowBackground 在非 Windows 平台直接返回错误。
func ShowBackground(dir string) error { return errUnsupported }
