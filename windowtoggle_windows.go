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

func findMainWindow() uintptr {
	title, err := syscall.UTF16PtrFromString(appTitle)
	if err != nil {
		return 0
	}
	h, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	return h
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
