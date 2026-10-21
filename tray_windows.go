//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// 托盘图标：内嵌的透明背景多尺寸 ICO（16/20/24/32/40/48/64）。
// 应用图标的 16px 帧是白底，直接放进任务栏会看成一块空白，所以单独裁了眼睛主体。
//
//go:embed build/windows/tray.ico
var trayIconICO []byte

// 系统托盘（任务栏右下角通知区域的小图标）。
//
// 说明：
//   - Wails v2 没有内建托盘支持，这里直接用 Shell_NotifyIconW 实现；
//   - 托盘窗口与消息循环跑在独立常驻线程，菜单弹出不会阻塞 WebView2 的 UI 线程；
//   - 左键单击托盘图标切换主窗口显隐，右键弹出「显示主窗口 / 退出程序」菜单；
//   - 资源管理器重启后会收到 TaskbarCreated 消息，自动重新添加图标。

var (
	shell32Tray = syscall.NewLazyDLL("shell32.dll")

	procShellNotifyIconW  = shell32Tray.NewProc("Shell_NotifyIconW")
	procRegisterClassW    = user32.NewProc("RegisterClassW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procPostMessageW      = user32.NewProc("PostMessageW")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procRegisterWindowMsg = user32.NewProc("RegisterWindowMessageW")
	procCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	procAppendMenuW       = user32.NewProc("AppendMenuW")
	procDestroyMenu       = user32.NewProc("DestroyMenu")
	procTrackPopupMenuEx  = user32.NewProc("TrackPopupMenuEx")
	procGetCursorPos      = user32.NewProc("GetCursorPos")
	procLoadIconW         = user32.NewProc("LoadIconW")
	procLoadImageW        = user32.NewProc("LoadImageW")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procDestroyIcon       = user32.NewProc("DestroyIcon")

	procGetModuleHandleW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	trayClassName = "KeeYGeeNTrayWnd"

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	trayCallbackMsg = 0x8000 + 1 // WM_APP + 1

	wmClose         = 0x0010
	wmNull          = 0x0000
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	mfString    = 0x00000000
	mfSeparator = 0x00000800

	tpmReturnCmd   = 0x0100
	tpmRightButton = 0x0002
	tpmBottomAlign = 0x0020

	wsPopup     = 0x80000000
	wsExToolWnd = 0x00000080

	imageIcon      = 1
	idiApplication = 32512
	lrLoadFromFile = 0x00000010
	smCxSmIcon     = 49
	smCySmIcon     = 50

	trayShowWindowCmd = 1
	trayQuitCmd       = 2
)

type trayPoint struct {
	x int32
	y int32
}

type trayMsgStruct struct {
	hwnd    uintptr
	message uint32
	_       uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	_       uint32
	pt      trayPoint
}

// trayWndClass 对应 WNDCLASSW。
type trayWndClass struct {
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
}

// trayNotifyIconData 对应 NOTIFYICONDATAW（V2 版本，cbSize 到 dwInfoFlags 为止）。
type trayNotifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
}

var (
	trayOnce        sync.Once
	trayWndMu       sync.Mutex
	trayWnd         uintptr
	currentTrayIcon uintptr

	taskbarCreatedMsg uint32
)

// trayLog 把托盘初始化过程中的异常写入临时日志，便于排查（正常时不产生文件以外的副作用）。
func trayLog(format string, args ...interface{}) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "dtidy-tray.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// startTray 在主程序启动后创建托盘图标（幂等）。
func startTray() {
	trayOnce.Do(func() {
		go runTrayThread()
	})
}

// stopTray 退出前删除托盘图标。
func stopTray() {
	trayWndMu.Lock()
	hwnd := trayWnd
	trayWndMu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

func runTrayThread() {
	defer func() {
		if r := recover(); r != nil {
			trayLog("panic: %v", r)
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString(trayClassName)
	titlePtr, _ := syscall.UTF16PtrFromString(appTitle)

	var wc trayWndClass
	wc.lpfnWndProc = trayWindowProc
	wc.hInstance = hInst
	wc.lpszClassName = className
	if r, _, e := procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		trayLog("RegisterClassW failed: %v", e)
		return
	}

	hwnd, _, e := procCreateWindowExW.Call(
		wsExToolWnd,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titlePtr)),
		wsPopup, 0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		trayLog("CreateWindowExW failed: %v", e)
		return
	}
	trayWndMu.Lock()
	trayWnd = hwnd
	trayWndMu.Unlock()
	trayLog("tray window created hwnd=%d", hwnd)

	if namePtr, err := syscall.UTF16PtrFromString("TaskbarCreated"); err == nil {
		r, _, _ := procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(namePtr)))
		taskbarCreatedMsg = uint32(r)
	}

	addTrayIcon(hwnd)

	var msg trayMsgStruct
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	trayWndMu.Lock()
	trayWnd = 0
	trayWndMu.Unlock()
}

var trayWindowProc = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) (ret uintptr) {
	defer func() {
		if recover() != nil {
			ret = 0
		}
	}()

	switch {
	case msg == uintptr(trayCallbackMsg):
		switch uint32(lparam) & 0xFFFF {
		case wmLButtonUp, wmLButtonDblClk:
			toggleMainWindow()
			return 0
		case wmRButtonUp:
			showTrayMenu(hwnd)
			return 0
		}
	case taskbarCreatedMsg != 0 && msg == uintptr(taskbarCreatedMsg):
		// 资源管理器重启后重新添加图标
		addTrayIcon(hwnd)
		return 0
	case msg == wmClose:
		removeTrayIcon(hwnd)
		procDestroyWindow.Call(hwnd)
		return 0
	case msg == 0x0002: // WM_DESTROY
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
})

// loadTrayIcon 优先使用内嵌的透明底图标；失败再回退到 exe 资源图标 / 系统默认图标。
func loadTrayIcon() uintptr {
	if h := loadEmbeddedTrayIcon(); h != 0 {
		return h
	}
	hInst, _, _ := procGetModuleHandleW.Call(0)
	if h, _, _ := procLoadImageW.Call(hInst, 1, imageIcon, 0, 0, 0); h != 0 {
		return h
	}
	if h, _, _ := procLoadIconW.Call(hInst, 1); h != 0 {
		return h
	}
	h, _, _ := procLoadIconW.Call(0, idiApplication)
	return h
}

// loadEmbeddedTrayIcon 把内嵌 ICO 写到临时文件，再用 LoadImageW（LR_LOADFROMFILE）读取，
// 不依赖 wails 对 exe 资源图标的编号方式。图标加载完成后临时文件即可删除。
func loadEmbeddedTrayIcon() uintptr {
	if len(trayIconICO) == 0 {
		return 0
	}
	p := filepath.Join(os.TempDir(), "dtidy-tray.ico")
	if err := os.WriteFile(p, trayIconICO, 0o644); err != nil {
		trayLog("write tray icon temp file failed: %v", err)
		return 0
	}
	defer os.Remove(p)

	pathPtr, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return 0
	}
	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	if cx == 0 {
		cx = 16
	}
	if cy == 0 {
		cy = 16
	}
	h, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(pathPtr)), imageIcon, cx, cy, lrLoadFromFile)
	return h
}

func addTrayIcon(hwnd uintptr) {
	var nid trayNotifyIconData
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = trayCallbackMsg
	nid.hIcon = loadTrayIcon()
	tip := syscall.StringToUTF16(appTitle)
	copy(nid.szTip[:], tip)

	ok, _, e := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if ok == 0 {
		// 图标已存在（例如资源管理器重启后重复注册）时改为更新
		ok, _, e = procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	}
	trayLog("Shell_NotifyIcon ok=%d err=%v hwnd=%d hIcon=%d", ok, e, hwnd, nid.hIcon)

	// 换图标时释放上一份，避免句柄泄漏
	if currentTrayIcon != 0 && currentTrayIcon != nid.hIcon {
		procDestroyIcon.Call(currentTrayIcon)
	}
	currentTrayIcon = nid.hIcon
}

func removeTrayIcon(hwnd uintptr) {
	var nid trayNotifyIconData
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

// showTrayMenu 在托盘图标位置弹出右键菜单。
func showTrayMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	showText, _ := syscall.UTF16PtrFromString("显示主窗口")
	quitText, _ := syscall.UTF16PtrFromString("退出程序")
	procAppendMenuW.Call(hMenu, mfString, trayShowWindowCmd, uintptr(unsafe.Pointer(showText)))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, mfString, trayQuitCmd, uintptr(unsafe.Pointer(quitText)))

	var pt trayPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// 微软文档要求：弹出菜单的 owner 必须是前台窗口，否则点击菜单外部不会消失。
	// 主窗口可见时优先用它做 owner；否则退回托盘隐藏窗口。
	owner := hwnd
	if w := findMainWindow(); w != 0 && isWindowVisible(w) && !isIconic(w) {
		owner = w
	}
	procSetForegroundWnd.Call(owner)

	cmd, _, _ := procTrackPopupMenuEx.Call(
		hMenu,
		tpmReturnCmd|tpmRightButton|tpmBottomAlign,
		uintptr(pt.x), uintptr(pt.y),
		owner, 0,
	)
	procPostMessageW.Call(owner, wmNull, 0, 0)

	switch cmd {
	case trayShowWindowCmd:
		showMainWindow()
	case trayQuitCmd:
		quitFromTray()
	}
}

// showMainWindow 无条件显示并置前主窗口。
func showMainWindow() {
	w := findMainWindow()
	if w == 0 {
		return
	}
	procShowWindow.Call(w, swShowNormal)
	procBringWindowToTop.Call(w)
	procSetForegroundWnd.Call(w)
}

func quitFromTray() {
	removeTrayIconFromThread()
	if appCtx != nil {
		wruntime.Quit(appCtx)
		return
	}
	os.Exit(0)
}

func removeTrayIconFromThread() {
	trayWndMu.Lock()
	hwnd := trayWnd
	trayWndMu.Unlock()
	if hwnd != 0 {
		removeTrayIcon(hwnd)
	}
}
