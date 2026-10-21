package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dtidy/internal/shellicon"
)

// RecentItem 是「最近」列表里的一条记录。
type RecentItem struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	Ext          string `json:"ext"`
	Kind         string `json:"kind"`
	Icon         string `json:"icon,omitempty"`
	Count        int    `json:"count"`
	LastUsed     int64  `json:"lastUsed"`
	LastUsedText string `json:"lastUsedText"`
	Exists       bool   `json:"exists"`
	Task         string `json:"task"`
}

// RecentGroup 是「最近」的一个分组（默认最近或自定义任务）。
type RecentGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// GetRecentGroups 返回「默认最近」+ 所有任务分组及其记录数。
func (a *App) GetRecentGroups() []RecentGroup {
	if a.recent == nil {
		return []RecentGroup{{ID: "", Name: "默认最近", Count: 0}}
	}
	counts := a.recent.TaskCounts()
	groups := []RecentGroup{{ID: "", Name: "默认最近", Count: counts[""]}}
	for _, t := range a.recent.Tasks() {
		groups = append(groups, RecentGroup{ID: t.ID, Name: t.Name, Count: counts[t.ID]})
	}
	return groups
}

// ListRecent 列出某个分组的记录（task 为空表示默认最近）。
func (a *App) ListRecent(task string) []RecentItem {
	if a.recent == nil {
		return []RecentItem{}
	}
	items := a.recent.Items(task)
	out := make([]RecentItem, 0, len(items))
	for _, it := range items {
		entry := RecentItem{
			Path:     it.Path,
			Name:     filepath.Base(it.Path),
			Ext:      strings.TrimPrefix(filepath.Ext(it.Path), "."),
			Count:    it.Count,
			LastUsed: it.LastUsed,
			Task:     it.Task,
		}
		entry.LastUsedText = recentTimeText(it.LastUsed)
		if st, err := os.Stat(it.Path); err == nil && !st.IsDir() {
			entry.Exists = true
			entry.Kind = kindOf(strings.TrimPrefix(filepath.Ext(it.Path), "."), false)
			entry.Icon = shellicon.PreviewFor(it.Path, false)
		} else {
			entry.Kind = "file"
		}
		out = append(out, entry)
	}
	return out
}

// MoveRecent 把记录移动到指定任务（空 = 默认最近）。
func (a *App) MoveRecent(path, task string) error {
	if a.recent == nil {
		return fmt.Errorf("最近记录未启用")
	}
	return a.recent.Move(path, task)
}

// RemoveRecent 只从最近记录里移除，不删除磁盘上的文件。
func (a *App) RemoveRecent(path string) error {
	if a.recent == nil {
		return fmt.Errorf("最近记录未启用")
	}
	a.recent.Remove(path)
	return nil
}

// CreateRecentTask 新建任务分组（不创建任何文件夹）。
func (a *App) CreateRecentTask(name string) error {
	if a.recent == nil {
		return fmt.Errorf("最近记录未启用")
	}
	_, err := a.recent.CreateTask(name)
	return err
}

// RenameRecentTask 重命名任务分组。
func (a *App) RenameRecentTask(id, name string) error {
	if a.recent == nil {
		return fmt.Errorf("最近记录未启用")
	}
	return a.recent.RenameTask(id, name)
}

// DeleteRecentTask 删除任务分组，组内记录回到「默认最近」。
func (a *App) DeleteRecentTask(id string) error {
	if a.recent == nil {
		return fmt.Errorf("最近记录未启用")
	}
	return a.recent.DeleteTask(id)
}

// recordOpen 记录一次打开（只记路径，不动文件）。
func (a *App) recordOpen(path string) {
	if a.recent == nil || !a.recentAllowed(path) {
		return
	}
	a.recent.Record(path)
}

// recentAllowed 过滤系统目录、临时文件与本程序自身，避免最近列表被噪音占满。
func (a *App) recentAllowed(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	low := strings.ToLower(filepath.Clean(path))
	if strings.HasSuffix(low, ".lnk") {
		return false
	}
	for _, bad := range []string{
		`\windows\`, `\program files`, `\programdata\`, `\appdata\`,
		`\temp\`, `\$recycle.bin\`, `\system32\`, `\systemvolumeinformation\`,
	} {
		if strings.Contains(low, bad) {
			return false
		}
	}
	if exe, err := os.Executable(); err == nil {
		if strings.EqualFold(filepath.Clean(exe), low) {
			return false
		}
	}
	return true
}

func recentTimeText(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	t := time.UnixMilli(ms)
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case t.After(midnight):
		return "今天 " + t.Format("15:04")
	case t.After(midnight.AddDate(0, 0, -1)):
		return "昨天 " + t.Format("15:04")
	case t.After(midnight.AddDate(0, 0, -7)):
		return t.Format("周一 15:04")
	default:
		return t.Format("2006-01-02 15:04")
	}
}
