// Package recent 记录「最近打开」的文件清单。
//
// 只保存路径与使用情况，不复制、不移动、不删除任何真实文件；
// 记录按「任务」分组：task 为空表示「默认最近」，其余为自定义任务。
package recent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dtidy/internal/config"
)

// 默认最近最多保留的条数，超出的丢弃最久未使用的。
const maxDefaultItems = 200

// Item 是一条最近打开记录。
type Item struct {
	Path     string `json:"path"`
	Task     string `json:"task"`     // 空 = 默认最近
	Count    int    `json:"count"`    // 打开次数
	LastUsed int64  `json:"lastUsed"` // Unix 毫秒
}

// Task 是一个任务分组（只存记录，不创建文件夹）。
type Task struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type snapshot struct {
	Tasks []Task `json:"tasks"`
	Items []Item `json:"items"`
}

// Store 是最近记录的持久化存储。
type Store struct {
	mu    sync.Mutex
	path  string
	snap  snapshot
	dirty int32
}

// New 读取配置目录下的 recent.json；不存在时返回空存储。
func New() (*Store, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "recent.json")}
	if data, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(data, &s.snap)
	}
	if s.snap.Tasks == nil {
		s.snap.Tasks = []Task{}
	}
	if s.snap.Items == nil {
		s.snap.Items = []Item{}
	}
	return s, nil
}

// Path 返回记录文件路径。
func (s *Store) Path() string { return s.path }

// Record 记录一次打开：同路径合并计数并刷新时间；已属于某任务时留在该任务。
func (s *Store) Record(path string) {
	path = cleanPath(path)
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	for i := range s.snap.Items {
		if strings.EqualFold(s.snap.Items[i].Path, path) {
			s.snap.Items[i].Count++
			s.snap.Items[i].LastUsed = now
			s.saveLocked()
			return
		}
	}
	s.snap.Items = append(s.snap.Items, Item{Path: path, Count: 1, LastUsed: now})
	s.trimDefaultLocked()
	s.saveLocked()
}

// Move 把记录移动到指定任务（task 为空表示移回「默认最近」）。
func (s *Store) Move(path, task string) error {
	path = cleanPath(path)
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if task != "" && !s.hasTaskLocked(task) {
		return fmt.Errorf("任务不存在")
	}
	for i := range s.snap.Items {
		if strings.EqualFold(s.snap.Items[i].Path, path) {
			s.snap.Items[i].Task = task
			s.trimDefaultLocked()
			s.saveLocked()
			return nil
		}
	}
	return fmt.Errorf("记录不存在")
}

// Remove 只删除记录，不删除磁盘上的文件。
func (s *Store) Remove(path string) {
	path = cleanPath(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.snap.Items[:0]
	for _, it := range s.snap.Items {
		if strings.EqualFold(it.Path, path) {
			continue
		}
		out = append(out, it)
	}
	s.snap.Items = out
	s.saveLocked()
}

// Items 返回某个任务下的记录，按最近使用时间倒序；task 为空表示默认最近。
func (s *Store) Items(task string) []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Item, 0, len(s.snap.Items))
	for _, it := range s.snap.Items {
		if it.Task == task {
			list = append(list, it)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].LastUsed > list[j].LastUsed })
	return list
}

// All 返回全部记录（供后台去重判断）。
func (s *Store) All() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Item, len(s.snap.Items))
	copy(out, s.snap.Items)
	return out
}

// Tasks 返回所有任务分组。
func (s *Store) Tasks() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, len(s.snap.Tasks))
	copy(out, s.snap.Tasks)
	return out
}

// TaskCounts 返回每个任务的记录数（key 为任务 ID，"" 为默认最近）。
func (s *Store) TaskCounts() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	for _, it := range s.snap.Items {
		counts[it.Task]++
	}
	return counts
}

// CreateTask 新建任务分组，返回新任务 ID。
func (s *Store) CreateTask(name string) (Task, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Task{}, fmt.Errorf("任务名不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.snap.Tasks {
		if strings.EqualFold(t.Name, name) {
			return Task{}, fmt.Errorf("任务「%s」已存在", name)
		}
	}
	t := Task{ID: fmt.Sprintf("t%d", time.Now().UnixNano()), Name: name}
	s.snap.Tasks = append(s.snap.Tasks, t)
	s.saveLocked()
	return t, nil
}

// RenameTask 重命名任务。
func (s *Store) RenameTask(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("任务名不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.snap.Tasks {
		if s.snap.Tasks[i].ID == id {
			for j, t := range s.snap.Tasks {
				if j != i && strings.EqualFold(t.Name, name) {
					return fmt.Errorf("任务「%s」已存在", name)
				}
			}
			s.snap.Tasks[i].Name = name
			s.saveLocked()
			return nil
		}
	}
	return fmt.Errorf("任务不存在")
}

// DeleteTask 删除任务分组：其中的记录移回「默认最近」，不删除任何文件。
func (s *Store) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, t := range s.snap.Tasks {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("任务不存在")
	}
	s.snap.Tasks = append(s.snap.Tasks[:idx], s.snap.Tasks[idx+1:]...)
	for i := range s.snap.Items {
		if s.snap.Items[i].Task == id {
			s.snap.Items[i].Task = ""
		}
	}
	s.trimDefaultLocked()
	s.saveLocked()
	return nil
}

// PruneMissing 清理已经不存在于磁盘的记录（可选调用）。
func (s *Store) PruneMissing() {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.snap.Items[:0]
	for _, it := range s.snap.Items {
		if st, err := os.Stat(it.Path); err == nil && !st.IsDir() {
			out = append(out, it)
		}
	}
	s.snap.Items = out
	s.saveLocked()
}

func (s *Store) hasTaskLocked(id string) bool {
	for _, t := range s.snap.Tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}

// trimDefaultLocked 限制「默认最近」的条数，丢弃最久未使用的。
func (s *Store) trimDefaultLocked() {
	n := 0
	for _, it := range s.snap.Items {
		if it.Task == "" {
			n++
		}
	}
	if n <= maxDefaultItems {
		return
	}
	drop := n - maxDefaultItems
	// 默认组里最旧的先删
	idxs := make([]int, 0, n)
	for i, it := range s.snap.Items {
		if it.Task == "" {
			idxs = append(idxs, i)
		}
	}
	sort.Slice(idxs, func(a, b int) bool {
		return s.snap.Items[idxs[a]].LastUsed < s.snap.Items[idxs[b]].LastUsed
	})
	remove := map[int]bool{}
	for i := 0; i < drop && i < len(idxs); i++ {
		remove[idxs[i]] = true
	}
	out := s.snap.Items[:0]
	for i, it := range s.snap.Items {
		if remove[i] {
			continue
		}
		out = append(out, it)
	}
	s.snap.Items = out
}

func (s *Store) saveLocked() {
	data, err := json.MarshalIndent(s.snap, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
	atomic.AddInt32(&s.dirty, 1)
}

func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}
