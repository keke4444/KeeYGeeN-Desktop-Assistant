//go:build windows

package main

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	user32Single     = syscall.NewLazyDLL("user32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
	procCloseHandle  = kernel32.NewProc("CloseHandle")
	procMessageBoxW  = user32Single.NewProc("MessageBoxW")
)

var singleInstanceMutex uintptr

// ensureSingleInstance 防止重复启动。
//
// 已经存在实例时：等待其主窗口出现（双击过快时第一个实例可能还没建好窗口），
// 无论窗口是正常显示还是隐藏在托盘，都把它唤到前台后退出。
// 即使找不到窗口也不再启动第二个实例——否则两个实例会争抢全局热键，
// 出现「热键已被占用」的提示。
func ensureSingleInstance() bool {
	name, err := syscall.UTF16PtrFromString(appTitle + "_single_instance")
	if err != nil {
		return true
	}
	h, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true
	}
	if errno, ok := callErr.(syscall.Errno); !ok || errno != 183 { // ERROR_ALREADY_EXISTS
		singleInstanceMutex = h
		return true
	}

	// 已有实例：最多等 6 秒让它的主窗口就绪
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if w := findMainWindow(); w != 0 {
			procShowWindow.Call(w, swShowNormal)
			procShowWindow.Call(w, swRestore)
			procSetForegroundWnd.Call(w)
			procCloseHandle.Call(h)
			return false
		}
		time.Sleep(120 * time.Millisecond)
	}

	// 找不到窗口：互斥体被没有窗口的残留进程占着。仍然退出，
	// 保证同一时间只有一个实例；提示用户从托盘图标唤出或清理残留进程。
	title, _ := syscall.UTF16PtrFromString(appTitle)
	text, _ := syscall.UTF16PtrFromString("程序已在运行，但主窗口未找到。\n请稍后从任务栏托盘图标中唤出，或结束残留进程后再试。")
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x00000040)
	procCloseHandle.Call(h)
	return false
}
