//go:build windows

package main

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procEnumWindows      = user32.NewProc("EnumWindows")
	procGetClassNameW    = user32.NewProc("GetClassNameW")
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procIsWindow         = user32.NewProc("IsWindow")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procIsIconic         = user32.NewProc("IsIconic")
	procGetForegroundWnd = user32.NewProc("GetForegroundWindow")
	procSetForegroundWnd = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop = user32.NewProc("BringWindowToTop")
)

const (
	swHide       = 0
	swShowNormal = 1
	swRestore    = 9

	// wails 主窗口的窗口类名；托盘隐藏窗口（KeeYGeeNTrayWnd）标题与主窗口相同，
	// 必须按类名区分，否则唤出时会作用到托盘窗口上导致主窗口依然隐藏。
	wailsWindowClass = "wailsWindow"
)

var (
	mainWndMu sync.Mutex
	mainWnd   uintptr
)

// toggleMainWindow 热键切换主窗口：可见且在前台时隐藏，否则显示并置前。
func toggleMainWindow() {
	mainWndMu.Lock()
	defer mainWndMu.Unlock()

	if mainWnd == 0 || !isWindow(mainWnd) {
		mainWnd = findMainWindow()
	}
	if mainWnd == 0 {
		return
	}

	if !isWindowVisible(mainWnd) || isIconic(mainWnd) {
		procShowWindow.Call(mainWnd, swRestore)
		procSetForegroundWnd.Call(mainWnd)
		return
	}

	fg, _, _ := procGetForegroundWnd.Call()
	if fg == mainWnd {
		procShowWindow.Call(mainWnd, swHide)
		return
	}
	procShowWindow.Call(mainWnd, swShowNormal)
	procBringWindowToTop.Call(mainWnd)
	procSetForegroundWnd.Call(mainWnd)
}

// findMainWindow 枚举顶层窗口，精确匹配 Wails 主窗口（类名 wailsWindow + 标题）。
// 之前用 FindWindowW(nil, appTitle) 按标题查找，但托盘隐藏窗口标题与主窗口相同，
// Z-order 变化时会命中托盘窗口，导致唤出失败（表现为"程序无法启动"）。
func findMainWindow() uintptr {
	if h := findWailsWindow(); h != 0 {
		return h
	}
	// 兜底：按标题查找（旧行为）
	title, err := syscall.UTF16PtrFromString(appTitle)
	if err != nil {
		return 0
	}
	h, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	return h
}

// findWailsWindow 遍历所有顶层窗口，找类名为 wailsWindow 且标题等于 appTitle 的窗口。
func findWailsWindow() uintptr {
	var found uintptr
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		var cls [64]uint16
		r, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&cls[0])), uintptr(len(cls)))
		if r == 0 || syscall.UTF16ToString(cls[:]) != wailsWindowClass {
			return 1
		}
		var title [512]uint16
		r, _, _ = procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		if r >= 0 && syscall.UTF16ToString(title[:]) == appTitle {
			found = hwnd
			return 0 // 找到后停止枚举
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return found
}

func isWindow(h uintptr) bool {
	r, _, _ := procIsWindow.Call(h)
	return r != 0
}

func isWindowVisible(h uintptr) bool {
	r, _, _ := procIsWindowVisible.Call(h)
	return r != 0
}

func isIconic(h uintptr) bool {
	r, _, _ := procIsIconic.Call(h)
	return r != 0
}
