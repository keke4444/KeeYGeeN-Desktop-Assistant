//go:build windows

// Package hotkey 注册 Windows 全局热键（RegisterHotKey），用于召唤主窗口。
package hotkey

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	wmHotkey    = 0x0312
	wmAppReload = 0x8001
	wmAppQuit   = 0x8002

	modAlt      = 0x0001
	modControl  = 0x0002
	modShift    = 0x0004
	modWin      = 0x0008
	modNoRepeat = 0x4000
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
)

type point struct{ X, Y int32 }

type msgStruct struct {
	hwnd     uintptr
	message  uint32
	_        uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

// Manager 管理一个全局热键；支持在运行中切换。
type Manager struct {
	mu       sync.Mutex
	spec     string
	threadID uint32
}

// Parse 解析 "Ctrl+Alt+D" 形式的热键，返回修饰键与虚拟键码。
func Parse(spec string) (mods uint32, vk uint32, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, fmt.Errorf("热键为空")
	}
	parts := strings.Split(spec, "+")
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("热键格式不正确")
	}
	key := ""
	for _, raw := range parts {
		p := strings.TrimSpace(raw)
		switch strings.ToLower(p) {
		case "ctrl", "control":
			mods |= modControl
		case "alt":
			mods |= modAlt
		case "shift":
			mods |= modShift
		case "win", "meta":
			mods |= modWin
		default:
			if key != "" {
				return 0, 0, fmt.Errorf("一个热键只能包含一个普通键")
			}
			key = p
		}
	}
	if key == "" {
		return 0, 0, fmt.Errorf("热键缺少普通键，例如 Ctrl+Alt+D")
	}
	code, ok := keyCode(key)
	if !ok {
		return 0, 0, fmt.Errorf("不支持的按键: %s", key)
	}
	if mods == 0 {
		return 0, 0, fmt.Errorf("热键需要至少一个修饰键（Ctrl/Alt/Shift/Win）")
	}
	return mods, code, nil
}

func keyCode(key string) (uint32, bool) {
	k := strings.ToUpper(key)
	if len(k) == 1 {
		c := k[0]
		switch {
		case c >= 'A' && c <= 'Z':
			return uint32(c), true
		case c >= '0' && c <= '9':
			return uint32(c), true
		}
	}
	if strings.HasPrefix(k, "F") {
		var n int
		if _, err := fmt.Sscanf(k, "F%d", &n); err == nil && n >= 1 && n <= 24 {
			return uint32(0x70 + n - 1), true
		}
	}
	named := map[string]uint32{
		"SPACE": 0x20, "ENTER": 0x0D, "RETURN": 0x0D, "TAB": 0x09,
		"ESC": 0x1B, "ESCAPE": 0x1B, "BACKSPACE": 0x08, "DELETE": 0x2E,
		"INSERT": 0x2D, "HOME": 0x24, "END": 0x23,
		"PAGEUP": 0x21, "PAGEDOWN": 0x22,
		"LEFT": 0x25, "UP": 0x26, "RIGHT": 0x27, "DOWN": 0x28,
	}
	v, ok := named[k]
	return v, ok
}

// Start 启动热键管理线程。spec 为空表示暂不启用。
func Start(spec string, onFire func()) (*Manager, error) {
	m := &Manager{spec: strings.TrimSpace(spec)}
	ready := make(chan error, 1)
	go m.loop(onFire, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) loop(onFire func(), ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := procGetCurrentThreadID.Call()
	m.mu.Lock()
	m.threadID = uint32(tid)
	spec := m.spec
	m.mu.Unlock()

	if err := registerSpec(spec); err != nil {
		ready <- err
		return
	}
	ready <- nil

	for {
		var msg msgStruct
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			unregisterAll()
			return
		}
		switch msg.message {
		case wmHotkey:
			if onFire != nil {
				go onFire()
			}
		case wmAppReload:
			unregisterAll()
			m.mu.Lock()
			next := m.spec
			m.mu.Unlock()
			_ = registerSpec(next)
		case wmAppQuit:
			unregisterAll()
			return
		}
	}
}

func (m *Manager) thread() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.threadID
}

// Set 更新热键（立即生效）。
func (m *Manager) Set(spec string) error {
	spec = strings.TrimSpace(spec)
	if spec != "" {
		if _, _, err := Parse(spec); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.spec = spec
	m.mu.Unlock()

	tid := m.thread()
	if tid == 0 {
		return fmt.Errorf("热键线程未就绪")
	}
	procPostThreadMessage.Call(uintptr(tid), wmAppReload, 0, 0)
	return nil
}

// Close 注销热键并结束管理线程。
func (m *Manager) Close() {
	tid := m.thread()
	if tid != 0 {
		procPostThreadMessage.Call(uintptr(tid), wmAppQuit, 0, 0)
	}
}

var (
	regMu       sync.Mutex
	regMods     uint32
	regKey      uint32
	regActive   bool
)

func registerSpec(spec string) error {
	if spec == "" {
		return nil
	}
	mods, vk, err := Parse(spec)
	if err != nil {
		return err
	}
	r, _, callErr := procRegisterHotKey.Call(0, 1, uintptr(mods|modNoRepeat), uintptr(vk))
	if r == 0 {
		return fmt.Errorf("注册热键 %s 失败（可能已被其它程序占用）: %v", spec, callErr)
	}
	regMu.Lock()
	regActive = true
	regMods, regKey = mods, vk
	regMu.Unlock()
	return nil
}

func unregisterAll() {
	regMu.Lock()
	active := regActive
	regActive = false
	regMu.Unlock()
	if active {
		procUnregisterHotKey.Call(0, 1)
	}
}
