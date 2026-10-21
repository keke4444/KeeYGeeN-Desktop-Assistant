//go:build windows

package recent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	watchShell32 = syscall.NewLazyDLL("shell32.dll")
	watchOle32   = syscall.NewLazyDLL("ole32.dll")

	procCoInitializeEx   = watchOle32.NewProc("CoInitializeEx")
	procCoCreateInstance = watchOle32.NewProc("CoCreateInstance")
)

// StartWatch 启动后台记录。
//
// Windows 没有公开的「文件被打开」事件，全局实时监听需要内核驱动或管理员权限的 ETW 会话；
// 本程序以普通用户权限运行，所以采用 1 秒级轮询系统「最近使用」目录（%APPDATA%\Microsoft\Windows\Recent）：
// 资源管理器或常用软件打开文件后系统会写入 .lnk，解析其目标即可同步到最近列表。
func (s *Store) StartWatch(filter func(string) bool) {
	go func() {
		defer func() { _ = recover() }()
		s.pollRecentFolder(filter)
	}()
}

// pollRecentFolder 轮询系统「最近使用」目录，把新出现的 .lnk 解析成真实文件路径后记录。
func (s *Store) pollRecentFolder(filter func(string) bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED

	base := os.Getenv("APPDATA")
	if base == "" {
		return
	}
	dir := filepath.Join(base, "Microsoft", "Windows", "Recent")
	last := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		now := time.Now()
		seen := map[string]bool{}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".lnk") {
				continue
			}
			info, err := e.Info()
			if err != nil || !info.ModTime().After(last) {
				continue
			}
			target := resolveShortcut(filepath.Join(dir, e.Name()))
			if target == "" || seen[strings.ToLower(target)] {
				continue
			}
			if st, err := os.Stat(target); err != nil || st.IsDir() {
				continue
			}
			seen[strings.ToLower(target)] = true
			if filter == nil || filter(target) {
				s.Record(target)
			}
		}
		last = now
	}
}

// resolveShortcut 用 IShellLink + IPersistFile 解析 .lnk 的目标路径。
func resolveShortcut(lnk string) string {
	rclsid := guid{0x00021401, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidLink := guid{0x000214F9, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	var link unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&rclsid)), 0, 1,
		uintptr(unsafe.Pointer(&iidLink)), uintptr(unsafe.Pointer(&link)),
	)
	if int32(hr) < 0 || link == nil {
		return ""
	}
	defer comCall(link, 2) // Release

	iidPersist := guid{0x0000010B, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	var persist unsafe.Pointer
	hr = comCall(link, 0, uintptr(unsafe.Pointer(&iidPersist)), uintptr(unsafe.Pointer(&persist)))
	if int32(hr) < 0 || persist == nil {
		return ""
	}
	defer comCall(persist, 2)

	pathPtr, err := syscall.UTF16PtrFromString(lnk)
	if err != nil {
		return ""
	}
	hr = comCall(persist, 5, uintptr(unsafe.Pointer(pathPtr)), 0) // IPersistFile::Load(STGM_READ)
	if int32(hr) < 0 {
		return ""
	}
	var buf [syscall.MAX_PATH]uint16
	hr = comCall(link, 3, uintptr(unsafe.Pointer(&buf[0])), syscall.MAX_PATH, 0, 0) // IShellLinkW::GetPath
	if int32(hr) < 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:])
}

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

func comCall(obj unsafe.Pointer, index uintptr, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, index*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(obj))
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}
