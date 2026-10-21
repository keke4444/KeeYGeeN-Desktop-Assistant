package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"dtidy/internal/candrule"
	"dtidy/internal/category"
	"dtidy/internal/config"
	"dtidy/internal/fsops"
	"dtidy/internal/hotkey"
	"dtidy/internal/journal"
	"dtidy/internal/platform"
	"dtidy/internal/recent"
	"dtidy/internal/shellicon"
	"dtidy/internal/shellmenu"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var version = "0.13.3"

// App 是暴露给前端的后端对象。
type App struct {
	ctx context.Context

	mu           sync.Mutex
	cfg          *config.Config
	cfgPath      string
	desktop      string
	root         string // 整理根：<桌面>\home
	j            *journal.Journal
	batch        string
	hk           *hotkey.Manager
	hkErr        string
	autoStartErr string
	recent       *recent.Store
}

// Settings 是界面设置，供前端读写。
type Settings struct {
	BgImage            string    `json:"bgImage"`
	BgName             string    `json:"bgName"`
	HasBackground      bool      `json:"hasBackground"`
	BgOpacity          float64   `json:"bgOpacity"`
	BgFit              string    `json:"bgFit"`
	BgScope            string    `json:"bgScope"`
	BgRect             []float64 `json:"bgRect"`
	CategoriesPosition string    `json:"categoriesPosition"`
	IconScale          float64   `json:"iconScale"`
	Hotkey             string    `json:"hotkey"`
	ThemeAccent        string    `json:"themeAccent"`
	ThemePage          string    `json:"themePage"`
	ThemePageCustom    bool      `json:"themePageCustom"`
	ThemeBack          string    `json:"themeBack"`
	FollowThemeBack    bool      `json:"followThemeBack"`
	ThemeRecent        string    `json:"themeRecent"`
	FollowRecent       bool      `json:"followRecent"`
	FontFamily         string    `json:"fontFamily"`
	FontSize           float64   `json:"fontSize"`
	FontColor          string    `json:"fontColor"`
	FontParts          map[string]FontPartSetting `json:"fontParts"`
	SearchCandidates   int       `json:"searchCandidates"`
	CandRuleFile       string    `json:"candRuleFile"`
	CandRuleName       string    `json:"candRuleName"`
	CandRuleError      string    `json:"candRuleError,omitempty"`
	AutoStart          bool      `json:"autoStart"`
	CloseToTaskbar     bool      `json:"closeToTaskbar"`
	DesktopPath        string    `json:"desktopPath"`
	HomePath           string    `json:"homePath"`
	HotkeyError        string    `json:"hotkeyError,omitempty"`
	AutoStartError     string    `json:"autoStartError,omitempty"`
}

// FontPartSetting 是某分区的字体覆盖（空值 = 继承全局）。
type FontPartSetting struct {
	Family string  `json:"family"`
	Size   float64 `json:"size"`
	Color  string  `json:"color"`
}

func fontPartsToSettings(in map[string]config.FontPart) map[string]FontPartSetting {
	out := make(map[string]FontPartSetting, len(in))
	for k, v := range in {
		out[k] = FontPartSetting{Family: v.Family, Size: v.Size, Color: v.Color}
	}
	return out
}

func fontPartsFromSettings(in map[string]FontPartSetting) map[string]config.FontPart {
	out := make(map[string]config.FontPart, len(in))
	for k, v := range in {
		out[k] = config.FontPart{Family: v.Family, Size: v.Size, Color: v.Color}
	}
	return out
}

// CatInfo 是侧栏里的一个分类。
type CatInfo struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Count int    `json:"count"`
	Icon  string `json:"icon,omitempty"`
}

// OpenItem 是侧栏里通过「打开文件夹」加入的目录。
type OpenItem struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Path string `json:"path"`
	Icon string `json:"icon,omitempty"`
}

// EntryInfo 是列表里的一个条目。
type EntryInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	SizeText string `json:"sizeText"`
	Ext      string `json:"ext"`
	Kind     string `json:"kind"`
	ModTime  string `json:"modTime"`
	IsDir    bool   `json:"isDir"`
	Icon     string `json:"icon,omitempty"`
}

// State 是应用级状态（与当前浏览目录无关）。
type State struct {
	Desktop      string     `json:"desktop"`
	Root         string     `json:"root"`
	DesktopName  string     `json:"desktopName"`
	DesktopIcon  string     `json:"desktopIcon,omitempty"`
	HomeName     string     `json:"homeName"`
	HomeIcon     string     `json:"homeIcon,omitempty"`
	OpenFolders  []OpenItem `json:"openFolders"`
	Categories   []CatInfo  `json:"categories"`
	SidebarWidth int        `json:"sidebarWidth"`
	ConfigPath   string     `json:"configPath"`
	JournalPath  string     `json:"journalPath"`
	Version      string     `json:"version"`
	Verify       string     `json:"verify"`
	Conflict     string     `json:"conflict"`
	LoadError    string     `json:"loadError,omitempty"`
}

// Listing 是某个目录的内容。
type Listing struct {
	Path    string      `json:"path"`
	Parent  string      `json:"parent"`
	Name    string      `json:"name"`
	IsRoot  bool        `json:"isRoot"`
	IsHome  bool        `json:"isHome"`
	Entries []EntryInfo `json:"entries"`
}

// ItemResult 是单个文件的移动结果。
type ItemResult struct {
	Name    string `json:"name"`
	Src     string `json:"src"`
	Dst     string `json:"dst"`
	Ok      bool   `json:"ok"`
	Skipped bool   `json:"skipped"`
	Reason  string `json:"reason,omitempty"`
}

// MoveReport 是一次批量移动的汇总。
type MoveReport struct {
	Total   int          `json:"total"`
	Ok      int          `json:"ok"`
	Skipped int          `json:"skipped"`
	Failed  int          `json:"failed"`
	Batch   string       `json:"batch"`
	Target  string       `json:"target"`
	Items   []ItemResult `json:"items"`
}

// UndoReport 是撤销汇总。
type UndoReport struct {
	Batch   string     `json:"batch"`
	Ok      int        `json:"ok"`
	Skipped int        `json:"skipped"`
	Failed  int        `json:"failed"`
	Items   []UndoItem `json:"items"`
}

// UndoItem 是单条撤销结果。
type UndoItem struct {
	Name   string `json:"name"`
	Ok     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// NewApp 初始化配置、桌面路径、整理根与日志。
func NewApp() (*App, error) {
	cfg, cfgPath, err := config.Load()
	if err != nil {
		return nil, err
	}
	desktop, err := resolveDesktop(cfg)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(desktop); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("桌面目录不可访问: %s", desktop)
	}

	root := desktop
	if cfg.RootDir != "" {
		if filepath.IsAbs(cfg.RootDir) {
			root = filepath.Clean(cfg.RootDir)
		} else {
			root = filepath.Join(desktop, cfg.RootDir)
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("创建整理根目录失败: %w", err)
	}

	j := journal.OpenAt(filepath.Join(os.TempDir(), "dtidy-journal.jsonl"))
	if real, err := journal.Open(); err == nil {
		j = real
	}

	rs, _ := recent.New()

	return &App{
		cfg:     cfg,
		cfgPath: cfgPath,
		desktop: desktop,
		root:    root,
		j:       j,
		batch:   newBatchID(),
		recent:  rs,
	}, nil
}

// hideOnClose 返回关闭窗口时是否隐藏到任务栏（而不是退出程序）。
func (a *App) hideOnClose() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.CloseToTaskbar
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 最近打开记录：程序内打开会直接记录；系统里打开的文件由后台监听补录
	if a.recent != nil {
		a.recent.StartWatch(a.recentAllowed)
	}
	// 配置里开启了开机启动时，启动过程顺便校正注册表里的可执行文件路径
	if a.cfg.AutoStart {
		if err := applyAutoStart(true); err != nil {
			a.autoStartErr = err.Error()
		}
	}
	if strings.TrimSpace(a.cfg.Hotkey) == "" {
		return
	}
	hk, err := hotkey.Start(a.cfg.Hotkey, func() {
		toggleMainWindow()
	})
	if err != nil {
		a.hkErr = err.Error()
		return
	}
	a.hk = hk
}

// GetState 返回应用级状态（分类列表等）。
func (a *App) GetState() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stateLocked()
}

func (a *App) stateLocked() State {
	st := State{
		Desktop:      a.desktop,
		Root:         a.root,
		DesktopName:  a.itemName("desktop", "桌面"),
		DesktopIcon:  a.itemIcon("desktop", ""),
		HomeName:     a.itemName("home", "home"),
		HomeIcon:     a.itemIcon("home", ""),
		SidebarWidth: a.cfg.SidebarWidth,
		ConfigPath:   a.cfgPath,
		JournalPath:  a.j.Path(),
		Version:      version,
		Verify:       a.cfg.Verify,
		Conflict:     a.cfg.Conflict,
	}

	for _, p := range a.cfg.CustomFolders {
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			continue
		}
		key := "open:" + p
		st.OpenFolders = append(st.OpenFolders, OpenItem{
			Key:  key,
			Name: a.itemName(key, filepath.Base(p)),
			Path: p,
			Icon: a.itemIcon(key, shellicon.FolderIcon()),
		})
	}

	reg, err := category.Build(a.root, a.cfg.DiscoverCategories,
		a.cfg.ExtraCategories, a.cfg.ExcludeCategories)
	if err != nil {
		st.LoadError = err.Error()
		return st
	}
	for _, c := range reg.Categories {
		if a.isHiddenCategory(c.Name) {
			continue
		}
		key := "cat:" + c.Name
		target := c.Path
		if v := strings.TrimSpace(a.cfg.CategoryTargets[c.Name]); v != "" {
			if fi, err := os.Stat(v); err == nil && fi.IsDir() {
				target = v
			}
		}
		count := 0
		if entries, err := os.ReadDir(target); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					count++
				}
			}
		}
		st.Categories = append(st.Categories, CatInfo{
			Key:   key,
			Name:  a.itemName(key, c.Name),
			Path:  target,
			Count: count,
			Icon:  a.itemIcon(key, shellicon.FolderIcon()),
		})
	}
	return st
}

// itemName 返回侧栏项的显示名称（支持重命名）。
func (a *App) itemName(key, def string) string {
	if v := strings.TrimSpace(a.cfg.ItemAliases[key]); v != "" {
		return v
	}
	return def
}

// itemIcon 返回侧栏项的自定义图标 data URL，未设置时返回默认图标。
func (a *App) itemIcon(key, def string) string {
	p := strings.TrimSpace(a.cfg.ItemIcons[key])
	if p == "" {
		return def
	}
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return def
	}
	return shellicon.PreviewFor(p, false)
}

func (a *App) isHiddenCategory(name string) bool {
	for _, n := range a.cfg.HiddenCategories {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// ListDir 列出指定目录；path 为空时返回桌面。
func (a *App) ListDir(path string) (Listing, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if strings.TrimSpace(path) == "" {
		path = a.desktop
	}
	path = filepath.Clean(path)

	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return Listing{}, fmt.Errorf("目录不存在: %s", path)
	}

	entries, err := fsops.List(path, a.cfg.ShowHidden, a.cfg.Ignore)
	if err != nil {
		return Listing{}, fmt.Errorf("读取目录失败: %w", err)
	}

	listing := Listing{
		Path:   path,
		Name:   filepath.Base(path),
		IsRoot: samePath(path, a.root),
		IsHome: samePath(path, filepath.Dir(a.root)),
	}
	if !samePath(path, a.desktop) {
		listing.Parent = filepath.Dir(path)
	}
	if listing.IsRoot {
		listing.Name = "home"
	}
	if samePath(path, a.desktop) {
		listing.Name = "桌面"
	}

	for _, e := range entries {
		listing.Entries = append(listing.Entries, EntryInfo{
			Name:     e.Name,
			Path:     e.Path,
			Size:     e.Size,
			SizeText: humanSize(e.Size),
			Ext:      strings.TrimPrefix(e.Ext, "."),
			Kind:     kindOf(e.Ext, e.IsDir),
			ModTime:  e.ModTime.Format("2006-01-02 15:04"),
			IsDir:    e.IsDir,
			Icon:     shellicon.PreviewFor(e.Path, e.IsDir),
		})
	}
	return listing, nil
}

// Search 按文件名搜索。dir 为空时搜索桌面（含 home）、已打开文件夹与分类指定目录；
// 传入 dir 时只搜索该目录。多个关键词用空格分隔，需要全部命中。
// 结果最多 200 条，遇到无权限目录直接跳过。
func (a *App) Search(query, dir string) ([]EntryInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	q := strings.ToLower(strings.TrimSpace(query))
	results := make([]EntryInfo, 0, 64)
	if q == "" {
		return results, nil
	}
	tokens := strings.Fields(q)
	const maxResults = 200
	rules := a.candRulesLocked()

	var roots []string
	if strings.TrimSpace(dir) != "" {
		d := filepath.Clean(dir)
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			return results, fmt.Errorf("目录不存在: %s", d)
		}
		roots = append(roots, d)
	} else {
		roots = append(roots, a.desktop)
		addRoot := func(p string) {
			if strings.TrimSpace(p) == "" || pathEqualOrUnder(p, a.desktop) {
				return
			}
			for _, r := range roots {
				if pathEqualOrUnder(p, r) {
					return
				}
			}
			roots = append(roots, p)
		}
		for _, p := range a.cfg.CustomFolders {
			addRoot(p)
		}
		for _, p := range a.cfg.CategoryTargets {
			addRoot(p)
		}
	}

	seen := map[string]bool{}
	for _, root := range roots {
		if len(results) >= maxResults {
			break
		}
		root := root
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if len(results) >= maxResults {
				return fs.SkipAll
			}
			if samePath(p, root) {
				return nil
			}
			name := d.Name()
			hidden := strings.HasPrefix(name, ".") && !a.cfg.ShowHidden
			if a.cfg.Ignored(name) || a.cfg.IsSystem(name) || hidden {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if rules != nil && rules.Excluded(name) {
				return nil // 目录命中也只是不展示，仍继续遍历
			}
			low := strings.ToLower(name)
			for _, t := range tokens {
				if !strings.Contains(low, t) {
					return nil
				}
			}
			key := strings.ToLower(filepath.Clean(p))
			if seen[key] {
				return nil
			}

			item := EntryInfo{Name: name, Path: p, IsDir: d.IsDir(), Kind: "folder"}
			if d.IsDir() {
				item.SizeText = "—"
				item.Icon = shellicon.PreviewFor(p, true)
			} else {
				fi, infoErr := d.Info()
				if infoErr != nil {
					return nil
				}
				ext := filepath.Ext(name)
				item.Size = fi.Size()
				item.SizeText = humanSize(fi.Size())
				item.Ext = strings.TrimPrefix(ext, ".")
				item.Kind = kindOf(ext, false)
				item.ModTime = fi.ModTime().Format("2006-01-02 15:04")
				item.Icon = shellicon.PreviewFor(p, false)
			}
			seen[key] = true
			results = append(results, item)
			return nil
		})
	}

	if rules != nil && len(rules.Priority) > 0 {
		sort.SliceStable(results, func(i, j int) bool {
			return rules.Rank(results[i].Name) < rules.Rank(results[j].Name)
		})
	}
	return results, nil
}

// pathEqualOrUnder 判断 p 是否等于 dir 或位于 dir 内部。
func pathEqualOrUnder(p, dir string) bool {
	pc := strings.ToLower(filepath.Clean(p))
	dc := strings.ToLower(filepath.Clean(dir))
	return pc == dc || strings.HasPrefix(pc, dc+string(os.PathSeparator))
}

// candRulesLocked 读取当前配置的候选规则文件；未配置或解析失败时返回 nil。
func (a *App) candRulesLocked() *candrule.Rules {
	if strings.TrimSpace(a.cfg.CandRuleFile) == "" {
		return nil
	}
	r, err := candrule.Load(a.cfg.CandRuleFile)
	if err != nil {
		return nil
	}
	return r
}

// MoveTo 把文件移入目标目录；目标只能是桌面或整理根内部。
func (a *App) MoveTo(paths []string, destDir string) (MoveReport, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	report := MoveReport{Batch: a.batch}
	if len(paths) == 0 {
		return report, fmt.Errorf("没有选中文件")
	}

	destDir = filepath.Clean(destDir)
	if !a.allowedDest(destDir) {
		return report, fmt.Errorf("目标目录不在允许范围内: %s", destDir)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return report, fmt.Errorf("创建目标目录失败: %w", err)
	}
	report.Target = destDir

	opt := fsops.Options{Verify: a.cfg.Verify, Conflict: a.cfg.Conflict}
	for _, p := range paths {
		p = filepath.Clean(p)
		base := filepath.Base(p)
		item := ItemResult{Name: base, Src: p}

		fi, err := os.Stat(p)
		switch {
		case err != nil:
			item.Reason = "文件不存在"
		case fi.IsDir():
			item.Reason = "只移动文件，文件夹请直接在资源管理器中处理"
		case !a.allowedSource(p):
			item.Reason = "只能整理桌面或 home 里的文件"
		case samePath(filepath.Dir(p), destDir):
			item.Reason = "已经在该目录中"
		default:
			res := fsops.MoveFile(p, destDir, opt)
			item.Dst = res.Dst
			item.Ok = res.Ok
			item.Skipped = res.Skipped
			item.Reason = res.Err
			_ = a.j.Append(journal.Record{
				Batch:  a.batch,
				Action: "move",
				Src:    res.Src,
				Dst:    res.Dst,
				Size:   res.Size,
				SHA256: res.SHA256,
				Verify: res.Verify,
				OK:     res.Ok,
				Err:    res.Err,
			})
		}

		switch {
		case item.Skipped:
			report.Skipped++
		case item.Ok:
			report.Ok++
		default:
			report.Failed++
		}
		report.Items = append(report.Items, item)
	}
	report.Total = len(paths)
	return report, nil
}

// Undo 撤销最近一个成功批次。
func (a *App) Undo() (UndoReport, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	report := UndoReport{}
	results, batch, err := journal.Undo(a.j, "", a.cfg.Verify)
	if err != nil {
		return report, err
	}
	report.Batch = batch
	for _, r := range results {
		item := UndoItem{Name: filepath.Base(r.Dst), Ok: r.Ok, Reason: r.Reason}
		switch {
		case r.Ok:
			report.Ok++
		case r.Skipped:
			report.Skipped++
		default:
			report.Failed++
		}
		report.Items = append(report.Items, item)
	}
	return report, nil
}

// CreateCategory 在 home 下新建分类文件夹。
func (a *App) CreateCategory(name string) (State, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	name = strings.TrimSpace(name)
	if err := category.ValidateName(name); err != nil {
		return State{}, err
	}
	dir := filepath.Join(a.root, name)
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return State{}, fmt.Errorf("分类「%s」已存在", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return State{}, fmt.Errorf("创建分类失败: %w", err)
	}
	return a.stateLocked(), nil
}

// OpenFolder 用资源管理器打开目录。
func (a *App) OpenFolder(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("目录不存在: %s", path)
	}
	return exec.Command("explorer", path).Start()
}

// RevealFile 在资源管理器中选中文件。
func (a *App) RevealFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("文件不存在: %s", path)
	}
	return exec.Command("explorer", "/select,", path).Start()
}

// OpenFile 用系统默认程序打开文件。
func (a *App) OpenFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("文件不存在: %s", path)
	}
	a.recordOpen(path)
	return exec.Command("cmd", "/c", "start", "", path).Start()
}

// GetSettings 返回界面设置。
func (a *App) GetSettings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settingsLocked()
}

func (a *App) settingsLocked() Settings {
	s := Settings{
		BgImage:            a.cfg.BgImage,
		BgOpacity:          a.cfg.BgOpacity,
		BgFit:              a.cfg.BgFit,
		BgScope:            a.cfg.BgScope,
		BgRect:             a.cfg.BgRect,
		CategoriesPosition: a.cfg.CategoriesPosition,
		IconScale:          a.cfg.IconScale,
		Hotkey:             a.cfg.Hotkey,
		ThemeAccent:        a.cfg.ThemeAccent,
		ThemePage:          a.cfg.ThemePage,
		ThemePageCustom:    a.cfg.ThemePageCustom,
		ThemeBack:          a.cfg.ThemeBack,
		FollowThemeBack:    a.cfg.FollowThemeBack,
		ThemeRecent:        a.cfg.ThemeRecent,
		FollowRecent:       a.cfg.FollowRecent,
		FontFamily:         a.cfg.FontFamily,
		FontSize:           a.cfg.FontSize,
		FontColor:          a.cfg.FontColor,
		FontParts:          fontPartsToSettings(a.cfg.FontParts),
		SearchCandidates:   a.cfg.SearchCandidates,
		CandRuleFile:       a.cfg.CandRuleFile,
		AutoStart:          a.cfg.AutoStart,
		CloseToTaskbar:     a.cfg.CloseToTaskbar,
		DesktopPath:        a.desktop,
		HomePath:           a.root,
		HotkeyError:        a.hkErr,
		AutoStartError:     a.autoStartErr,
	}
	if a.cfg.BgImage != "" {
		if _, err := os.Stat(a.cfg.BgImage); err == nil {
			s.HasBackground = true
			s.BgName = filepath.Base(a.cfg.BgImage)
		}
	}
	if s.CandRuleFile != "" {
		s.CandRuleName = filepath.Base(s.CandRuleFile)
		if _, err := candrule.Load(s.CandRuleFile); err != nil {
			s.CandRuleError = err.Error()
		}
	}
	return s
}

// SaveSettings 保存界面设置（同时尝试立即更新热键）。
func (a *App) SaveSettings(in Settings) (Settings, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if in.Hotkey != "" {
		if _, _, err := hotkey.Parse(in.Hotkey); err != nil {
			return a.settingsLocked(), err
		}
	}
	a.cfg.BgOpacity = clampF(in.BgOpacity, 0.05, 1)
	a.cfg.BgFit = in.BgFit
	a.cfg.BgScope = in.BgScope
	a.cfg.BgRect = in.BgRect
	a.cfg.CategoriesPosition = in.CategoriesPosition
	a.cfg.IconScale = clampF(in.IconScale, 0.6, 2)
	a.cfg.Hotkey = strings.TrimSpace(in.Hotkey)
	a.cfg.ThemeAccent = in.ThemeAccent
	a.cfg.ThemePage = in.ThemePage
	a.cfg.ThemePageCustom = in.ThemePageCustom
	a.cfg.ThemeBack = in.ThemeBack
	a.cfg.FollowThemeBack = in.FollowThemeBack
	a.cfg.ThemeRecent = in.ThemeRecent
	a.cfg.FollowRecent = in.FollowRecent
	a.cfg.FontFamily = in.FontFamily
	a.cfg.FontSize = in.FontSize
	a.cfg.FontColor = in.FontColor
	a.cfg.FontParts = fontPartsFromSettings(in.FontParts)
	a.cfg.SearchCandidates = in.SearchCandidates
	a.cfg.AutoStart = in.AutoStart
	a.cfg.CloseToTaskbar = in.CloseToTaskbar
	a.cfg.NormalizeUI()

	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.settingsLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	if err := applyAutoStart(a.cfg.AutoStart); err != nil {
		a.autoStartErr = err.Error()
	} else {
		a.autoStartErr = ""
	}

	if a.hk != nil {
		if err := a.hk.Set(a.cfg.Hotkey); err != nil {
			a.hkErr = err.Error()
		} else {
			a.hkErr = ""
		}
	} else if a.cfg.Hotkey != "" {
		hk, err := hotkey.Start(a.cfg.Hotkey, func() {
			toggleMainWindow()
		})
		if err != nil {
			a.hkErr = err.Error()
		} else {
			a.hk = hk
			a.hkErr = ""
		}
	}
	return a.settingsLocked(), nil
}

// ChooseBackground 弹出文件选择框并保存背景图。
func (a *App) ChooseBackground() (Settings, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择背景图片",
		Filters: []runtime.FileFilter{
			{DisplayName: "图片 (*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp)", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp"},
		},
	})
	if err != nil {
		return a.settingsLocked(), err
	}
	if path == "" {
		return a.settingsLocked(), nil
	}
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return a.settingsLocked(), fmt.Errorf("无法读取该文件")
	}
	if fi.Size() > 40<<20 {
		return a.settingsLocked(), fmt.Errorf("背景图超过 40 MB，请换一张小一点的")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.BgImage = path
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.settingsLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.settingsLocked(), nil
}

// ClearBackground 清除自定义背景。
func (a *App) ClearBackground() (Settings, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.BgImage = ""
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.settingsLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.settingsLocked(), nil
}

// BackgroundURL 返回背景图的 data URL（gif/webp 动图会保留动画）。
func (a *App) BackgroundURL() string {
	a.mu.Lock()
	path := a.cfg.BgImage
	a.mu.Unlock()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || len(data) > 40<<20 {
		return ""
	}
	mime := "image/png"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".gif":
		mime = "image/gif"
	case ".webp":
		mime = "image/webp"
	case ".bmp":
		mime = "image/bmp"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// ShowShellMenu 弹出 Windows 原生右键菜单（与资源管理器一致）。
func (a *App) ShowShellMenu(paths []string) error {
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return fmt.Errorf("没有可操作的文件")
	}
	return shellmenu.Show(clean)
}

// ShowShellMenuBackground 弹出当前文件夹空白处的 Windows 原生右键菜单。
func (a *App) ShowShellMenuBackground(dir string) error {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" {
		return fmt.Errorf("目录为空")
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("目录不可用: %s", dir)
	}
	return shellmenu.ShowBackground(dir)
}

// ChooseOpenFolder 通过「打开文件夹」把目录加入侧栏（持久化）。
func (a *App) ChooseOpenFolder() (State, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "打开文件夹"})
	if err != nil {
		return a.GetState(), err
	}
	if strings.TrimSpace(path) == "" {
		return a.GetState(), nil
	}
	dir := filepath.Clean(path)
	if fi, statErr := os.Stat(dir); statErr != nil || !fi.IsDir() {
		return a.GetState(), fmt.Errorf("所选路径不是有效文件夹")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	exists := false
	for _, p := range a.cfg.CustomFolders {
		if samePath(p, dir) {
			exists = true
			break
		}
	}
	if !exists {
		a.cfg.CustomFolders = append(a.cfg.CustomFolders, dir)
	}
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.stateLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.stateLocked(), nil
}

// RemoveOpenFolder 仅关闭侧栏里的文件夹，不删除磁盘上的目录。
func (a *App) RemoveOpenFolder(key string) (State, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path := strings.TrimPrefix(key, "open:")
	next := make([]string, 0, len(a.cfg.CustomFolders))
	for _, p := range a.cfg.CustomFolders {
		if !samePath(p, path) {
			next = append(next, p)
		}
	}
	a.cfg.CustomFolders = next
	delete(a.cfg.ItemAliases, key)
	delete(a.cfg.ItemIcons, key)
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.stateLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.stateLocked(), nil
}

// SetItemAlias 设置侧栏项显示名称；传空字符串恢复默认。
func (a *App) SetItemAlias(key, alias string) (State, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	alias = strings.TrimSpace(alias)
	if alias == "" {
		delete(a.cfg.ItemAliases, key)
	} else {
		a.cfg.ItemAliases[key] = alias
	}
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.stateLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.stateLocked(), nil
}

// ChooseItemIcon 为侧栏项选择自定义图标。
func (a *App) ChooseItemIcon(key string) (State, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择图标图片",
		Filters: []runtime.FileFilter{
			{DisplayName: "图片 (*.png;*.jpg;*.jpeg;*.ico;*.bmp;*.gif;*.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.ico;*.bmp;*.gif;*.webp"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return a.GetState(), err
	}
	if strings.TrimSpace(path) == "" {
		return a.GetState(), nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.ItemIcons[key] = filepath.Clean(path)
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.stateLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.stateLocked(), nil
}

// RetargetItem 修改侧栏项指向的文件夹（桌面 / home / 分类 / 打开的文件夹）。
func (a *App) RetargetItem(key string) (State, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "指定文件夹"})
	if err != nil {
		return a.GetState(), err
	}
	if strings.TrimSpace(path) == "" {
		return a.GetState(), nil
	}
	dir := filepath.Clean(path)
	if fi, statErr := os.Stat(dir); statErr != nil || !fi.IsDir() {
		return a.GetState(), fmt.Errorf("所选路径不是有效文件夹")
	}

	// home 需要确认是否创建 home 子目录（在加锁前弹窗）
	if key == "home" && !strings.EqualFold(filepath.Base(dir), "home") {
		choice, dlgErr := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:          runtime.QuestionDialog,
			Title:         "创建 home 文件夹",
			Message:       fmt.Sprintf("所选文件夹「%s」不叫 home。\n是否在该文件夹下创建 home，并作为分类存放位置？", filepath.Base(dir)),
			Buttons:       []string{"创建 home", "取消"},
			DefaultButton: "创建 home",
			CancelButton:  "取消",
		})
		if dlgErr != nil {
			return a.GetState(), dlgErr
		}
		if choice != "创建 home" {
			return a.GetState(), nil
		}
		dir = filepath.Join(dir, "home")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return a.GetState(), fmt.Errorf("创建 home 失败: %w", err)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	switch {
	case key == "desktop":
		root := filepath.Join(dir, "home")
		if err := os.MkdirAll(root, 0o755); err != nil {
			return a.stateLocked(), fmt.Errorf("创建 home 失败: %w", err)
		}
		a.desktop = dir
		a.root = root
		a.cfg.Desktop = dir
		a.cfg.RootDir = root

	case key == "home":
		a.root = dir
		a.cfg.RootDir = dir

	case strings.HasPrefix(key, "cat:"):
		name := strings.TrimPrefix(key, "cat:")
		a.cfg.CategoryTargets[name] = dir

	case strings.HasPrefix(key, "open:"):
		old := strings.TrimPrefix(key, "open:")
		next := make([]string, 0, len(a.cfg.CustomFolders)+1)
		for _, p := range a.cfg.CustomFolders {
			if samePath(p, old) {
				continue
			}
			if samePath(p, dir) {
				continue
			}
			next = append(next, p)
		}
		next = append(next, dir)
		a.cfg.CustomFolders = next
		newKey := "open:" + dir
		if v, ok := a.cfg.ItemAliases[key]; ok {
			delete(a.cfg.ItemAliases, key)
			a.cfg.ItemAliases[newKey] = v
		}
		if v, ok := a.cfg.ItemIcons[key]; ok {
			delete(a.cfg.ItemIcons, key)
			a.cfg.ItemIcons[newKey] = v
		}

	default:
		return a.stateLocked(), fmt.Errorf("不支持的侧栏项: %s", key)
	}

	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.stateLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.stateLocked(), nil
}

// DeleteCategory 删除分类文件夹（含其中的文件），删除前弹窗确认。
func (a *App) DeleteCategory(key string) (State, error) {
	name := strings.TrimPrefix(key, "cat:")
	if err := category.ValidateName(name); err != nil {
		return a.GetState(), err
	}
	dir := filepath.Join(a.root, name)
	if samePath(dir, a.root) || !pathEqualOrUnder(dir, a.root) {
		return a.GetState(), fmt.Errorf("分类路径非法")
	}

	choice, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "删除分类",
		Message:       fmt.Sprintf("确定删除分类「%s」吗？\n\n该分类文件夹及其中所有文件都会被删除，且无法恢复。", name),
		Buttons:       []string{"删除", "取消"},
		DefaultButton: "取消",
		CancelButton:  "取消",
	})
	if err != nil {
		return a.GetState(), err
	}
	if choice != "删除" {
		return a.GetState(), nil
	}
	return a.deleteCategoryLocked(name)
}

// deleteCategoryLocked 执行实际的删除与配置清理（调用前需已确认）。
func (a *App) deleteCategoryLocked(name string) (State, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	dir := filepath.Join(a.root, name)
	if samePath(dir, a.root) || !pathEqualOrUnder(dir, a.root) {
		return a.stateLocked(), fmt.Errorf("分类路径非法")
	}

	if fi, statErr := os.Stat(dir); statErr == nil && fi.IsDir() {
		if err := os.RemoveAll(dir); err != nil {
			return a.stateLocked(), fmt.Errorf("删除分类失败: %w", err)
		}
	}

	// 清理该分类相关的侧栏配置
	delete(a.cfg.CategoryTargets, name)
	delete(a.cfg.ItemAliases, "cat:"+name)
	delete(a.cfg.ItemIcons, "cat:"+name)
	if a.isHiddenCategory(name) {
		next := make([]string, 0, len(a.cfg.HiddenCategories))
		for _, n := range a.cfg.HiddenCategories {
			if !strings.EqualFold(n, name) {
				next = append(next, n)
			}
		}
		a.cfg.HiddenCategories = next
	}
	if len(a.cfg.ExtraCategories) > 0 {
		next := make([]string, 0, len(a.cfg.ExtraCategories))
		for _, n := range a.cfg.ExtraCategories {
			if !strings.EqualFold(n, name) {
				next = append(next, n)
			}
		}
		a.cfg.ExtraCategories = next
	}
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.stateLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.stateLocked(), nil
}

// RemoveSidebarItem 从侧栏移除分类（不删磁盘文件）或关闭打开的文件夹。
func (a *App) RemoveSidebarItem(key string) (State, error) {
	switch {
	case strings.HasPrefix(key, "open:"):
		return a.RemoveOpenFolder(key)
	case strings.HasPrefix(key, "cat:"):
		name := strings.TrimPrefix(key, "cat:")
		a.mu.Lock()
		defer a.mu.Unlock()
		if !a.isHiddenCategory(name) {
			a.cfg.HiddenCategories = append(a.cfg.HiddenCategories, name)
		}
		if err := a.cfg.Save(a.cfgPath); err != nil {
			return a.stateLocked(), fmt.Errorf("保存配置失败: %w", err)
		}
		return a.stateLocked(), nil
	}
	return a.GetState(), fmt.Errorf("该项不能从侧栏移除")
}

// SaveSidebarWidth 保存侧栏宽度（拖动分隔条后调用）。
func (a *App) SaveSidebarWidth(width int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if width < 180 {
		width = 180
	}
	if width > 520 {
		width = 520
	}
	a.cfg.SidebarWidth = width
	return a.cfg.Save(a.cfgPath)
}

// ChooseCandidateRules 选择候选规则文件（支持 txt/json/csv/tsv/ini/conf/toml）。
func (a *App) ChooseCandidateRules() (Settings, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择候选规则文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "规则文件 (*.txt;*.json;*.csv;*.tsv;*.ini;*.conf;*.toml)", Pattern: "*.txt;*.json;*.csv;*.tsv;*.ini;*.conf;*.toml"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return a.GetSettings(), err
	}
	if strings.TrimSpace(path) == "" {
		return a.GetSettings(), nil
	}
	if _, err := candrule.Load(path); err != nil {
		return a.GetSettings(), fmt.Errorf("规则文件无法解析: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.CandRuleFile = filepath.Clean(path)
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.settingsLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.settingsLocked(), nil
}

// ClearCandidateRules 清除候选规则文件。
func (a *App) ClearCandidateRules() (Settings, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.CandRuleFile = ""
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.settingsLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.settingsLocked(), nil
}

// ChooseDesktop 选择桌面目录；home 自动使用该目录下的 home（不存在则创建）。
func (a *App) ChooseDesktop() (Settings, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择桌面文件夹"})
	if err != nil {
		return a.GetSettings(), err
	}
	if strings.TrimSpace(path) == "" {
		return a.GetSettings(), nil
	}
	dir := filepath.Clean(path)
	if fi, statErr := os.Stat(dir); statErr != nil || !fi.IsDir() {
		return a.GetSettings(), fmt.Errorf("所选路径不是有效文件夹")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if samePath(dir, a.desktop) {
		return a.settingsLocked(), nil
	}
	root := filepath.Join(dir, "home")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return a.settingsLocked(), fmt.Errorf("创建 home 失败: %w", err)
	}
	a.desktop = dir
	a.root = root
	a.cfg.Desktop = dir
	a.cfg.RootDir = root
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.settingsLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.settingsLocked(), nil
}

// ChooseHome 选择分类存放目录；若所选文件夹不叫 home，会先询问是否在其中创建 home。
func (a *App) ChooseHome() (Settings, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择 home 文件夹（分类存放位置）"})
	if err != nil {
		return a.GetSettings(), err
	}
	if strings.TrimSpace(path) == "" {
		return a.GetSettings(), nil
	}
	picked := filepath.Clean(path)
	if fi, statErr := os.Stat(picked); statErr != nil || !fi.IsDir() {
		return a.GetSettings(), fmt.Errorf("所选路径不是有效文件夹")
	}

	root := picked
	if !strings.EqualFold(filepath.Base(picked), "home") {
		choice, dlgErr := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:          runtime.QuestionDialog,
			Title:         "创建 home 文件夹",
			Message:       fmt.Sprintf("所选文件夹「%s」不叫 home。\n是否在该文件夹下创建 home，并作为分类存放位置？", filepath.Base(picked)),
			Buttons:       []string{"创建 home", "取消"},
			DefaultButton: "创建 home",
			CancelButton:  "取消",
		})
		if dlgErr != nil {
			return a.GetSettings(), dlgErr
		}
		if choice != "创建 home" {
			return a.GetSettings(), nil
		}
		root = filepath.Join(picked, "home")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return a.GetSettings(), fmt.Errorf("创建 home 失败: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.root = root
	a.cfg.RootDir = root
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.settingsLocked(), fmt.Errorf("保存配置失败: %w", err)
	}
	return a.settingsLocked(), nil
}

const (
	autostartKey  = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	autostartName = "dtidy"
)

// applyAutoStart 通过注册表 HKCU\...\Run 控制开机自启。
func applyAutoStart(on bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位程序路径: %w", err)
	}
	if on {
		if err := exec.Command("reg", "add", autostartKey, "/v", autostartName,
			"/t", "REG_SZ", "/d", `"`+exe+`"`, "/f").Run(); err != nil {
			return fmt.Errorf("写入开机启动项失败: %w", err)
		}
		return nil
	}
	// 删除不存在的项会返回非零，忽略该情况
	_ = exec.Command("reg", "delete", autostartKey, "/v", autostartName, "/f").Run()
	return nil
}

// RestartApp 重启应用（用于让热键等设置彻底生效）。
func (a *App) RestartApp() {
	exe, err := os.Executable()
	if err == nil {
		_ = exec.Command(exe).Start()
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (a *App) allowedDest(dir string) bool {
	return samePath(dir, a.desktop) || samePath(dir, a.root) ||
		strings.HasPrefix(strings.ToLower(dir), strings.ToLower(a.root)+string(os.PathSeparator)) ||
		a.extraPathAllowed(dir)
}

func (a *App) allowedSource(p string) bool {
	parent := filepath.Dir(p)
	return samePath(parent, a.desktop) || samePath(parent, a.root) ||
		strings.HasPrefix(strings.ToLower(parent), strings.ToLower(a.root)+string(os.PathSeparator)) ||
		a.extraPathAllowed(parent)
}

// extraPathAllowed 允许操作「打开文件夹」与分类指定文件夹内部。
func (a *App) extraPathAllowed(p string) bool {
	p = filepath.Clean(p)
	low := strings.ToLower(p)
	check := func(dir string) bool {
		if dir == "" {
			return false
		}
		d := strings.ToLower(filepath.Clean(dir))
		return low == d || strings.HasPrefix(low, d+string(os.PathSeparator))
	}
	for _, dir := range a.cfg.CustomFolders {
		if check(dir) {
			return true
		}
	}
	for _, dir := range a.cfg.CategoryTargets {
		if check(dir) {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func resolveDesktop(cfg *config.Config) (string, error) {
	if v := strings.TrimSpace(cfg.Desktop); v != "" {
		p := os.ExpandEnv(v)
		fi, err := os.Stat(p)
		if err != nil || !fi.IsDir() {
			return "", fmt.Errorf("配置的 desktop 不是有效目录: %s", p)
		}
		return p, nil
	}
	p, err := platform.DesktopDir()
	if err != nil {
		return "", fmt.Errorf("%w（可在配置文件中手动指定 desktop）", err)
	}
	return p, nil
}

func newBatchID() string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

func humanSize(n int64) string {
	const unit = int64(1024)
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := unit, 0
	for n/div >= unit && exp < 4 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTP"[exp])
}

func kindOf(ext string, isDir bool) string {
	if isDir {
		return "folder"
	}
	e := strings.TrimPrefix(strings.ToLower(ext), ".")
	switch e {
	case "png", "jpg", "jpeg", "gif", "bmp", "webp", "svg", "ico", "tif", "tiff", "heic", "psd", "ai", "xcf":
		return "image"
	case "mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "m4v", "mpg", "mpeg", "rmvb":
		return "video"
	case "mp3", "flac", "wav", "aac", "ogg", "m4a", "wma", "opus":
		return "audio"
	case "zip", "rar", "7z", "tar", "gz", "tgz", "bz2", "xz", "zst", "iso", "cab":
		return "archive"
	case "exe", "msi", "msix", "bat", "cmd", "ps1", "apk", "deb", "rpm":
		return "exe"
	case "go", "js", "ts", "tsx", "jsx", "py", "java", "c", "h", "cpp", "hpp", "cs", "rb", "php", "rs", "sh", "sql", "json", "yaml", "yml", "toml", "xml", "html", "css", "vue", "ipynb":
		return "code"
	case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "md", "csv", "rtf", "odt", "epub":
		return "doc"
	case "lnk", "url":
		return "link"
	}
	return "file"
}
