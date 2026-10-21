package category

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Category 是桌面上的一个分类文件夹：Name 即目录名，Path 是绝对路径。
type Category struct {
	Name string
	Path string
}

// Registry 是本次会话使用的分类列表（有序，编号稳定）。
type Registry struct {
	Categories []Category
}

// Names 返回分类名列表。
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.Categories))
	for _, c := range r.Categories {
		out = append(out, c.Name)
	}
	return out
}

// Get 按名称查找分类（Windows 语义：不区分大小写）。
func (r *Registry) Get(name string) (Category, bool) {
	for _, c := range r.Categories {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return Category{}, false
}

// Build 组装分类列表：指定根目录下的子文件夹（可选）+ 配置补充，去重后逐个校验命名。
// rootDir 是整理根（默认 <桌面>\home），所有分类都位于其下。
func Build(rootDir string, discover bool, extra, exclude []string) (*Registry, error) {
	var names []string
	if discover {
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("读取分类根目录失败: %w", err)
			}
			entries = nil
		}
		var dirs []string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dirs = append(dirs, e.Name())
		}
		sort.Slice(dirs, func(i, j int) bool {
			return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j])
		})
		names = append(names, dirs...)
	}
	names = append(names, extra...)

	// 排除（不区分大小写）
	excluded := map[string]bool{}
	for _, n := range exclude {
		excluded[strings.ToLower(strings.TrimSpace(n))] = true
	}

	reg := &Registry{}
	seen := map[string]bool{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if excluded[strings.ToLower(name)] {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		if err := ValidateName(name); err != nil {
			return nil, err
		}
		seen[key] = true
		reg.Categories = append(reg.Categories, Category{
			Name: name,
			Path: filepath.Join(rootDir, name),
		})
	}
	return reg, nil
}

// reserved 是 Windows 不允许用作文件/目录名的设备名。
var reserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// ValidateName 按 Windows 文件夹命名规则校验分类名。
// 非法时直接返回错误，工具不做任何改名映射。
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("分类名不能为空")
	}
	if len([]rune(name)) > 255 {
		return fmt.Errorf("分类名 %q 过长（超过 255 字符）", name)
	}
	for _, r := range name {
		if r < 0x20 {
			return fmt.Errorf("分类名 %q 含控制字符，Windows 不允许", name)
		}
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return fmt.Errorf("分类名 %q 含非法字符 %q（Windows 文件夹名不允许）", name, string(r))
		}
	}
	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("分类名 %q 不能以空格或点结尾（Windows 不允许）", name)
	}
	base := name
	if i := strings.Index(base, "."); i >= 0 {
		base = base[:i]
	}
	if reserved[strings.ToUpper(strings.TrimSpace(base))] {
		return fmt.Errorf("分类名 %q 是 Windows 保留设备名，不能用作文件夹名", name)
	}
	return nil
}
