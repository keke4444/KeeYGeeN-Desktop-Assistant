//go:build !windows

package hotkey

import "fmt"

// Manager 在非 Windows 平台为空实现。
type Manager struct{}

// Parse 在非 Windows 平台不支持热键。
func Parse(spec string) (uint32, uint32, error) {
	return 0, 0, fmt.Errorf("当前平台不支持全局热键")
}

// Start 在非 Windows 平台返回空管理器。
func Start(spec string, onFire func()) (*Manager, error) {
	return &Manager{}, nil
}

// Set 在非 Windows 平台不生效。
func (m *Manager) Set(spec string) error { return nil }

// Close 在非 Windows 平台无操作。
func (m *Manager) Close() {}
