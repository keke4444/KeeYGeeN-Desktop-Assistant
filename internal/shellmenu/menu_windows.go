//go:build windows

// Package shellmenu 调用 Windows Shell 的原生右键菜单（与资源管理器一致）。
//
// 稳定性要点：
//   - 菜单全部在独立的常驻 STA 线程上创建与弹出，绝不阻塞 Wails/WebView2 的 UI 线程；
//   - 弹出菜单前必须让宿主窗口成为前台窗口（微软文档要求），拿不到前台就取消，
//     由前端回退内置菜单，避免菜单卡在屏幕上；
//   - 每个请求有 recover 保护，单个 Shell 扩展异常不会拖垮进程；
//   - 菜单打开超过 45 秒会发送 WM_CANCELMODE 自动收起。
package shellmenu

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	ole32    = syscall.NewLazyDLL("ole32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCoInitializeEx     = ole32.NewProc("CoInitializeEx")
	procCoUninitialize     = ole32.NewProc("CoUninitialize")
	procCoTaskMemFree      = ole32.NewProc("CoTaskMemFree")
	procSHParseDisplayName = shell32.NewProc("SHParseDisplayName")
	procSHBindToParent     = shell32.NewProc("SHBindToParent")
	procCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	procDestroyMenu        = user32.NewProc("DestroyMenu")
	procTrackPopupMenuEx   = user32.NewProc("TrackPopupMenuEx")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procRegisterClassW     = user32.NewProc("RegisterClassW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procSetForegroundWnd   = user32.NewProc("SetForegroundWindow")
	procGetForegroundWnd   = user32.NewProc("GetForegroundWindow")
	procShowWindow         = user32.NewProc("ShowWindow")
	procPostMessageW       = user32.NewProc("PostMessageW")
)

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

var (
	iidIShellFolder  = guid{0x000214E6, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIShellView    = guid{0x000214E3, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIContextMenu  = guid{0x000214E4, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIContextMenu2 = guid{0x000214F4, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIContextMenu3 = guid{0xBCFCE0A0, 0xEC17, 0x11CF, [8]byte{0x8D, 0x10, 0x00, 0xAA, 0x00, 0x68, 0x0F, 0x33}}
)

const (
	tpmReturnCmd    = 0x0100
	tpmRightButton  = 0x0002
	wmInitMenuPopup = 0x0117
	wmDrawItem      = 0x002B
	wmMeasureItem   = 0x002C
	wmMenuChar      = 0x0078
	wmCancelMode    = 0x001F
	wmNull          = 0x0000
	swHide          = 0
	swShowNa        = 8
	swShowNormal    = 1

	wsPopup     = 0x80000000
	wsExToolWnd = 0x00000080

	menuTimeout = 45 * time.Second
)

type point struct {
	x int32
	y int32
}

type wndClassW struct {
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

type cmInvokeCommandInfoEx struct {
	cbSize        uint32
	fMask         uint32
	hwnd          uintptr
	lpVerb        uintptr
	lpParameters  uintptr
	lpDirectory   uintptr
	nShow         int32
	dwHotKey      uint32
	hIcon         uintptr
	lpTitle       uintptr
	lpVerbW       uintptr
	lpParametersW uintptr
	lpDirectoryW  uintptr
	lpTitleW      uintptr
	ptInvoke      point
}

type parsedItem struct {
	pidl  unsafe.Pointer
	psf   unsafe.Pointer
	child unsafe.Pointer
}

type menuRequest struct {
	paths []string // 文件/文件夹路径；dir 为空时使用
	dir   string   // 非空表示当前文件夹的背景菜单
	done  chan error
}

var (
	menuHostMu sync.Mutex
	hostICM2   unsafe.Pointer
	hostICM3   unsafe.Pointer
	hostClass  = "dtidyShellMenuHost"

	menuOnce  sync.Once
	menuReq   chan menuRequest
	menuAlive int32 // 菜单是否正在显示
)

// comCall 调用 COM 虚表方法。
func comCall(obj unsafe.Pointer, index uintptr, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, index*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(obj))
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func comRelease(obj unsafe.Pointer) {
	if obj != nil {
		comCall(obj, 2)
	}
}

var wndProc = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) (ret uintptr) {
	// Shell 扩展可能抛异常，回调里必须兜住，避免进程直接崩掉
	defer func() {
		if recover() != nil {
			ret = 0
		}
	}()
	switch msg {
	case wmInitMenuPopup, wmDrawItem, wmMeasureItem, wmMenuChar:
		menuHostMu.Lock()
		icm3 := hostICM3
		icm2 := hostICM2
		menuHostMu.Unlock()
		if icm3 != nil {
			var lr uintptr
			hr := comCall(icm3, 7, msg, wparam, lparam, uintptr(unsafe.Pointer(&lr)))
			if int32(hr) >= 0 {
				return lr
			}
		}
		if icm2 != nil {
			comCall(icm2, 6, msg, wparam, lparam)
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
})

// createHostWindow 在菜单线程上创建 1x1 离屏宿主窗口（菜单 owner）。
func createHostWindow() (uintptr, error) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	cls, err := syscall.UTF16PtrFromString(hostClass)
	if err != nil {
		return 0, err
	}
	wc := wndClassW{lpfnWndProc: wndProc, hInstance: hInst, lpszClassName: cls}
	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	title, _ := syscall.UTF16PtrFromString("")
	hwnd, _, callErr := procCreateWindowExW.Call(
		wsExToolWnd, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)),
		wsPopup, uintptr(0xFFFFFFF8), uintptr(0xFFFFFFF8), 1, 1, 0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("创建菜单宿主窗口失败: %v", callErr)
	}
	return hwnd, nil
}

func ensureMenuThread() {
	menuOnce.Do(func() {
		menuReq = make(chan menuRequest)
		go menuLoop()
	})
}

func menuLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	procCoInitializeEx.Call(0, 2 /* COINIT_APARTMENTTHREADED */)
	defer procCoUninitialize.Call()

	hwnd, err := createHostWindow()
	for req := range menuReq {
		if err != nil {
			req.done <- err
			continue
		}
		req.done <- safeRun(hwnd, req)
	}
}

// safeRun 包一层 recover，单个扩展出错只影响这一次菜单。
func safeRun(hwnd uintptr, req menuRequest) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("右键菜单执行异常: %v", r)
		}
	}()
	if req.dir != "" {
		return runBackgroundMenu(hwnd, req.dir)
	}
	return runFileMenu(hwnd, req.paths)
}

func dispatch(req menuRequest) error {
	ensureMenuThread()
	req.done = make(chan error, 1)
	// 队列最多等 3 秒，避免上一个菜单异常时把前端一直挂住
	select {
	case menuReq <- req:
	case <-time.After(3 * time.Second):
		return fmt.Errorf("菜单线程忙碌")
	}
	// 整体上限 60 秒（45 秒安全阀会自动取消菜单），保证前端状态能复位
	select {
	case err := <-req.done:
		return err
	case <-time.After(60 * time.Second):
		return fmt.Errorf("菜单响应超时")
	}
}

// Show 弹出文件的 Shell 原生右键菜单，支持同一目录下的多选。
func Show(paths []string) error {
	return dispatch(menuRequest{paths: paths})
}

// ShowBackground 弹出文件夹空白处的原生右键菜单（与资源管理器背景右键一致）。
func ShowBackground(dir string) error {
	return dispatch(menuRequest{dir: dir})
}

// parseOne 解析路径为 PIDL，并绑定父文件夹。
func parseOne(path string) (parsedItem, bool) {
	p16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return parsedItem{}, false
	}
	var pidl unsafe.Pointer
	hr, _, _ := procSHParseDisplayName.Call(
		uintptr(unsafe.Pointer(p16)), 0,
		uintptr(unsafe.Pointer(&pidl)), 0, 0,
	)
	if int32(hr) < 0 || pidl == nil {
		return parsedItem{}, false
	}
	var psf, child unsafe.Pointer
	hr, _, _ = procSHBindToParent.Call(
		uintptr(pidl),
		uintptr(unsafe.Pointer(&iidIShellFolder)),
		uintptr(unsafe.Pointer(&psf)),
		uintptr(unsafe.Pointer(&child)),
	)
	if int32(hr) < 0 || psf == nil || child == nil {
		procCoTaskMemFree.Call(uintptr(pidl))
		return parsedItem{}, false
	}
	return parsedItem{pidl: pidl, psf: psf, child: child}, true
}

func runFileMenu(hwnd uintptr, paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("没有选中文件")
	}

	items := make([]parsedItem, 0, len(paths))
	for _, p := range paths {
		if it, ok := parseOne(p); ok {
			items = append(items, it)
		}
	}
	defer func() {
		for _, it := range items {
			procCoTaskMemFree.Call(uintptr(it.pidl))
			comRelease(it.psf)
		}
	}()
	if len(items) == 0 {
		return fmt.Errorf("无法解析选中的文件")
	}

	// 只有所有选中项位于同一目录时才作为多选菜单
	sameDir := true
	dir0 := strings.ToLower(filepathDir(paths[0]))
	for _, p := range paths[1:] {
		if strings.ToLower(filepathDir(p)) != dir0 {
			sameDir = false
			break
		}
	}

	count := 1
	if sameDir {
		count = len(items)
	}
	children := make([]unsafe.Pointer, 0, count)
	for i := 0; i < count; i++ {
		children = append(children, items[i].child)
	}
	parent := items[0].psf

	// IShellFolder::GetUIObjectOf(hwnd, cidl, apidl, riid, reserved, &out)
	var pcm unsafe.Pointer
	hr := comCall(parent, 10,
		hwnd, uintptr(len(children)),
		uintptr(unsafe.Pointer(&children[0])),
		uintptr(unsafe.Pointer(&iidIContextMenu)),
		0,
		uintptr(unsafe.Pointer(&pcm)),
	)
	if int32(hr) < 0 || pcm == nil {
		return fmt.Errorf("获取右键菜单失败: 0x%x", hr)
	}
	defer comRelease(pcm)

	return popup(pcm, hwnd)
}

func runBackgroundMenu(hwnd uintptr, dir string) error {
	it, ok := parseOne(dir)
	if !ok {
		return fmt.Errorf("无法解析文件夹: %s", dir)
	}
	defer procCoTaskMemFree.Call(uintptr(it.pidl))
	defer comRelease(it.psf)

	// 父文件夹 -> 目标文件夹的 IShellFolder（BindToObject，虚表下标 5）
	var folder unsafe.Pointer
	hr := comCall(it.psf, 5,
		uintptr(it.child), 0,
		uintptr(unsafe.Pointer(&iidIShellFolder)),
		uintptr(unsafe.Pointer(&folder)),
	)
	if int32(hr) < 0 || folder == nil {
		return fmt.Errorf("打开文件夹失败: 0x%x", hr)
	}
	defer comRelease(folder)

	// IShellFolder::CreateViewObject -> IShellView::GetItemObject(SVGIO_BACKGROUND)
	var psv unsafe.Pointer
	hr = comCall(folder, 8,
		hwnd,
		uintptr(unsafe.Pointer(&iidIShellView)),
		uintptr(unsafe.Pointer(&psv)),
	)
	if int32(hr) < 0 || psv == nil {
		return fmt.Errorf("获取文件夹视图失败: 0x%x", hr)
	}
	defer comRelease(psv)

	var pcm unsafe.Pointer
	hr = comCall(psv, 15,
		0, // SVGIO_BACKGROUND
		uintptr(unsafe.Pointer(&iidIContextMenu)),
		uintptr(unsafe.Pointer(&pcm)),
	)
	if int32(hr) < 0 || pcm == nil {
		return fmt.Errorf("获取文件夹背景菜单失败: 0x%x", hr)
	}
	defer comRelease(pcm)

	return popup(pcm, hwnd)
}

// popup 创建并弹出菜单，执行用户选择的命令。
func popup(pcm unsafe.Pointer, hwnd uintptr) error {
	// 优先 IContextMenu3，其次 IContextMenu2，用于子菜单/自绘项的消息处理
	var pcm3, pcm2 unsafe.Pointer
	if hr := comCall(pcm, 0,
		uintptr(unsafe.Pointer(&iidIContextMenu3)),
		uintptr(unsafe.Pointer(&pcm3))); int32(hr) < 0 {
		pcm3 = nil
	}
	if pcm3 == nil {
		if hr := comCall(pcm, 0,
			uintptr(unsafe.Pointer(&iidIContextMenu2)),
			uintptr(unsafe.Pointer(&pcm2))); int32(hr) < 0 {
			pcm2 = nil
		}
	}

	menuHostMu.Lock()
	hostICM3 = pcm3
	hostICM2 = pcm2
	menuHostMu.Unlock()
	defer func() {
		menuHostMu.Lock()
		hostICM3 = nil
		hostICM2 = nil
		menuHostMu.Unlock()
		if pcm3 != nil {
			comRelease(pcm3)
		}
		if pcm2 != nil {
			comRelease(pcm2)
		}
	}()

	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return fmt.Errorf("创建菜单失败")
	}
	defer procDestroyMenu.Call(hMenu)

	// IContextMenu::QueryContextMenu(hmenu, indexMenu, idCmdFirst, idCmdLast, uFlags)
	hr := comCall(pcm, 3, hMenu, 0, 1, 0x7FF, 0)
	if int32(hr) < 0 {
		return fmt.Errorf("生成右键菜单失败: 0x%x", hr)
	}

	// 微软文档要求：调用 TrackPopupMenuEx 前 owner 必须是前台窗口，
	// 否则点击菜单外部时菜单不会消失；拿不到前台就不弹，交给前端回退内置菜单。
	prevFg, _, _ := procGetForegroundWnd.Call()
	procShowWindow.Call(hwnd, swShowNa)
	procSetForegroundWnd.Call(hwnd)
	if cur, _, _ := procGetForegroundWnd.Call(); cur != hwnd {
		procShowWindow.Call(hwnd, swHide)
		return fmt.Errorf("无法激活菜单窗口，已取消原生菜单")
	}

	// 安全阀：菜单异常长时间不返回时自动取消
	atomic.StoreInt32(&menuAlive, 1)
	go func(h uintptr) {
		time.Sleep(menuTimeout)
		if atomic.LoadInt32(&menuAlive) == 1 {
			procPostMessageW.Call(h, wmCancelMode, 0, 0)
		}
	}(hwnd)

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	cmd, _, _ := procTrackPopupMenuEx.Call(
		hMenu,
		tpmReturnCmd|tpmRightButton,
		uintptr(pt.x), uintptr(pt.y),
		hwnd, 0,
	)
	atomic.StoreInt32(&menuAlive, 0)

	// 收尾：让菜单彻底结束，并隐藏宿主窗口
	procPostMessageW.Call(hwnd, wmNull, 0, 0)
	procShowWindow.Call(hwnd, swHide)
	if prevFg != 0 && prevFg != hwnd {
		procSetForegroundWnd.Call(prevFg)
	}
	if cmd == 0 {
		return nil
	}

	info := cmInvokeCommandInfoEx{
		cbSize: uint32(unsafe.Sizeof(cmInvokeCommandInfoEx{})),
		hwnd:   hwnd,
		lpVerb: cmd - 1, // MAKEINTRESOURCE
		nShow:  swShowNormal,
	}
	hr = comCall(pcm, 4, uintptr(unsafe.Pointer(&info))) // IContextMenu::InvokeCommand
	if int32(hr) < 0 {
		return fmt.Errorf("执行菜单命令失败: 0x%x", hr)
	}
	return nil
}

// filepathDir 避免在此文件引入 path/filepath（只需取父目录字符串）。
func filepathDir(p string) string {
	i := strings.LastIndexAny(p, `\/`)
	if i <= 0 {
		return p
	}
	return p[:i]
}
