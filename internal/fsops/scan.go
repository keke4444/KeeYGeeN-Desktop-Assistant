package fsops

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry 是目录里的一个条目（文件或子目录）。
type Entry struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
	Ext     string // 小写，含点；无扩展名为 ""
	IsDir   bool
	Marked  bool
}

// ScanResult 区分可整理文件与子目录。
type ScanResult struct {
	Files []Entry
	Dirs  []string
}

// Scan 扫描目录顶层条目（不递归）；Files 只含普通文件，Dirs 只含目录名。
func Scan(dir string, showHidden bool, ignore []string) (ScanResult, error) {
	entries, err := List(dir, showHidden, ignore)
	if err != nil {
		return ScanResult{}, err
	}
	var res ScanResult
	for _, e := range entries {
		if e.IsDir {
			res.Dirs = append(res.Dirs, e.Name)
			continue
		}
		res.Files = append(res.Files, e)
	}
	return res, nil
}

// List 列出目录内容：目录在前、文件在后，各自按名称排序。
func List(dir string, showHidden bool, ignore []string) ([]Entry, error) {
	raw, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	ignored := make(map[string]bool, len(ignore))
	for _, n := range ignore {
		ignored[strings.ToLower(n)] = true
	}

	var dirs, files []Entry
	for _, e := range raw {
		name := e.Name()
		lower := strings.ToLower(name)
		if ignored[lower] || strings.HasPrefix(lower, ".dtidy-tmp-") {
			continue
		}
		switch lower {
		case "desktop.ini", "thumbs.db", ".ds_store":
			continue
		}
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}
		entry := Entry{
			Name:    name,
			Path:    filepath.Join(dir, name),
			Ext:     strings.ToLower(filepath.Ext(name)),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime(),
		}
		if e.IsDir() {
			dirs = append(dirs, entry)
		} else {
			entry.Size = info.Size()
			files = append(files, entry)
		}
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return append(dirs, files...), nil
}
