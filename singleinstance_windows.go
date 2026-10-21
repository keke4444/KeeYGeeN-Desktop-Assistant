//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
	procCloseHandle  = kernel32.NewProc("CloseHandle")
)

var singleInstanceMutex uintptr

// ensureSingleInstance 防止重复启动：已有可见实例时唤出它并返回 false。
// 若检测到的旧实例没有可见窗口（异常残留），则继续启动，避免应用打不开。
func ensureSingleInstance() bool {
	name, err := syscall.UTF16PtrFromString(appTitle + "_single_instance")
	if err != nil {
		return true
	}
	h, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h != 0 {
		// Call 的第三个返回值就是 CreateMutexW 之后的 GetLastError
		if errno, ok := callErr.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
			if w := findMainWindow(); w != 0 && isWindowVisible(w) {
				procShowWindow.Call(w, swRestore)
				procSetForegroundWnd.Call(w)
				procCloseHandle.Call(h)
				return false
			}
		}
		singleInstanceMutex = h
	}
	return true
}
