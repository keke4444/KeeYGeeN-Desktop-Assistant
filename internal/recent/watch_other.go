//go:build !windows

package recent

// StartWatch 在非 Windows 平台暂不提供系统级「最近打开」监听，
// 程序内打开的文件仍会正常记录。
func (s *Store) StartWatch(filter func(string) bool) {}
