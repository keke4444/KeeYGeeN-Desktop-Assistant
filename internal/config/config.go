package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config 是 dtidy 的全部可配置项。字段名与 docs/DESIGN.md §5 一致。
type Config struct {
	Desktop            string   `toml:"desktop"`
	RootDir            string   `toml:"root_dir"`
	DiscoverCategories bool     `toml:"discover_categories"`
	ExtraCategories    []string `toml:"extra_categories"`
	ExcludeCategories  []string `toml:"exclude_categories"`
	Conflict           string   `toml:"conflict"`
	Verify             string   `toml:"verify"`
	Concurrency        int      `toml:"concurrency"`
	ShowHidden         bool     `toml:"show_hidden"`
	Ignore             []string `toml:"ignore"`
	ConfirmAboveMB     int64    `toml:"confirm_above_mb"`

	// 界面与交互
	BgImage            string    `toml:"bg_image"`            // 背景图片绝对路径
	BgOpacity          float64   `toml:"bg_opacity"`          // 0-1
	BgFit              string    `toml:"bg_fit"`              // contain | cover | stretch | tile
	BgScope            string    `toml:"bg_scope"`            // window | main | custom
	BgRect             []float64 `toml:"bg_rect"`             // 自定义作用域 x,y,w,h（窗口比例 0-1）
	CategoriesPosition string    `toml:"categories_position"` // left | right | top
	SidebarWidth       int       `toml:"sidebar_width"`       // 侧栏宽度（180-520）
	IconScale          float64   `toml:"icon_scale"`          // 0.6-2.0
	Hotkey             string    `toml:"hotkey"`              // 例如 Ctrl+Alt+D，空=不启用
	SearchCandidates   int       `toml:"search_candidates"`   // 搜索候选下拉的条数，3-30
	CandRuleFile       string    `toml:"cand_rule_file"`      // 候选规则文件路径（可空）
	AutoStart          bool      `toml:"autostart"`           // 开机自动启动
	CloseToTaskbar     bool      `toml:"close_to_taskbar"`    // 关闭窗口时隐藏到任务栏而非退出

	// 侧栏自定义项
	CustomFolders    []string          `toml:"custom_folders"`    // 「打开文件夹」加入的目录
	ItemAliases      map[string]string `toml:"item_aliases"`      // key -> 显示名称
	ItemIcons        map[string]string `toml:"item_icons"`        // key -> 图标文件
	CategoryTargets  map[string]string `toml:"category_targets"`  // 分类名 -> 指定文件夹
	HiddenCategories []string          `toml:"hidden_categories"` // 从侧栏隐藏的分类

	// 配色
	ThemeAccent     string `toml:"theme_accent"`      // 整体颜色（强调色）
	ThemePage       string `toml:"theme_page"`        // 页面背景色
	ThemePageCustom bool   `toml:"theme_page_custom"` // 页面背景是否已单独指定（否则跟随整体颜色）
	ThemeBack       string `toml:"theme_back"`        // 返回上一级按钮颜色（需关闭跟随才生效）
	FollowThemeBack bool   `toml:"follow_theme_back"` // 返回按钮是否跟随整体颜色
	ThemeRecent     string `toml:"theme_recent"`     // 最近任务标签颜色（需关闭跟随时生效）
	FollowRecent    bool   `toml:"follow_recent"`    // 最近任务标签是否跟随整体颜色

	// 字体
	FontFamily string              `toml:"font_family"` // 空 = 系统默认字体
	FontSize   float64             `toml:"font_size"`   // 0 = 默认字号 13.5
	FontColor  string              `toml:"font_color"`  // 空 = 跟随主题文字颜色
	FontParts  map[string]FontPart `toml:"font_parts"`  // 分区覆盖：sidebar / pathbar / file / settings
}

// FontPart 是某个界面分区的字体覆盖（空值表示继承全局）。
type FontPart struct {
	Family string  `toml:"family"`
	Size   float64 `toml:"size"`
	Color  string  `toml:"color"`
}

var (
	// DefaultAccent / DefaultBgOpacityPct / DefaultBgFit 可通过 ldflags 定制不同发行版的外观默认值。
	DefaultAccent       = "#e95420"
	DefaultBgOpacityPct = "35"
	DefaultBgFit        = "contain"
)

// Default 返回内置默认值：桌面下建 home 作为整理根，分类放在 home 里。
func Default() Config {
	opacity := 0.35
	if v, err := strconv.Atoi(DefaultBgOpacityPct); err == nil && v > 0 {
		opacity = float64(v) / 100
	}
	return Config{
		Desktop:            "",
		RootDir:            "home",
		DiscoverCategories: true,
		ExtraCategories:    []string{},
		ExcludeCategories:  []string{},
		Conflict:           "index",
		Verify:             "sha256",
		Concurrency:        4,
		ShowHidden:         false,
		Ignore:             []string{"desktop.ini", "Thumbs.db", ".DS_Store"},
		ConfirmAboveMB:     0,
		BgImage:            "",
		BgOpacity:          opacity,
		BgFit:              DefaultBgFit,
		BgScope:            "main",
		BgRect:             []float64{0, 0, 1, 1},
		CategoriesPosition: "left",
		SidebarWidth:       258,
		IconScale:          1.0,
		Hotkey:             "Ctrl+Alt+D",
		SearchCandidates:   8,
		AutoStart:          false,
		CloseToTaskbar:     false,
		CandRuleFile:       "",
		CustomFolders:      []string{},
		ItemAliases:        map[string]string{},
		ItemIcons:          map[string]string{},
		CategoryTargets:    map[string]string{},
		HiddenCategories:   []string{},
		ThemeAccent:        DefaultAccent,
		ThemePage:          "#2c001e",
		ThemePageCustom:    false,
		ThemeBack:          "",
		FollowThemeBack:    true,
		ThemeRecent:        "",
		FollowRecent:       true,
		FontFamily:         "",
		FontSize:           0,
		FontColor:          "",
		FontParts:          map[string]FontPart{},
	}
}

// DirName 是配置目录名；可通过 -ldflags "-X dtidy/internal/config.DirName=xxx" 区分不同版本。
var DirName = "dtidy"

// DefaultBgFile 是随程序附带的可选默认背景文件名；存在时作为初始背景。
var DefaultBgFile = "bg.png"

// UseDefaultBg 为 "true" 时自动使用 exe 同目录的 DefaultBgFile 作为初始背景。
// 正式版保持空值（不带预设图片）；只在定制发行版（发给姐姐的第二版）通过
// -ldflags "-X dtidy/internal/config.UseDefaultBg=true" 打开。
var UseDefaultBg = ""

// Dir 返回配置目录（不保证存在）。
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法定位配置目录: %w", err)
	}
	return filepath.Join(base, DirName), nil
}

// Path 返回配置文件绝对路径。
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load 读取配置；文件不存在时自动写入默认配置。返回配置与配置文件路径。
func Load() (*Config, string, error) {
	p, err := Path()
	if err != nil {
		return nil, "", err
	}
	cfg := Default()
	if _, statErr := os.Stat(p); statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return nil, p, statErr
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, p, fmt.Errorf("创建配置目录失败: %w", err)
		}
		if err := cfg.Save(p); err != nil {
			return nil, p, fmt.Errorf("写入默认配置失败: %w", err)
		}
	} else {
		if _, err := toml.DecodeFile(p, &cfg); err != nil {
			return nil, p, fmt.Errorf("解析配置 %s 失败: %w", p, err)
		}
	}

	applyEnv(&cfg)
	applyDefaultBg(&cfg)
	cfg.Normalize()
	cfg.NormalizeUI()
	if err := cfg.Validate(); err != nil {
		return nil, p, err
	}
	return &cfg, p, nil
}

// applyDefaultBg 在启用预设背景时，使用 exe 同目录下的 DefaultBgFile（安装包附带）。
func applyDefaultBg(cfg *Config) {
	if !strings.EqualFold(strings.TrimSpace(UseDefaultBg), "true") {
		return
	}
	if strings.TrimSpace(cfg.BgImage) != "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	p := filepath.Join(filepath.Dir(exe), DefaultBgFile)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		cfg.BgImage = p
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("DTIDY_DESKTOP"); v != "" {
		cfg.Desktop = v
	}
	if v := os.Getenv("DTIDY_ROOT"); v != "" {
		cfg.RootDir = v
	}
}

// Normalize 统一小写、去重、去空白。
func (c *Config) Normalize() {
	c.Conflict = strings.ToLower(strings.TrimSpace(c.Conflict))
	c.Verify = strings.ToLower(strings.TrimSpace(c.Verify))
	seen := map[string]bool{}
	ignore := make([]string, 0, len(c.Ignore))
	for _, n := range c.Ignore {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		key := strings.ToLower(n)
		if seen[key] {
			continue
		}
		seen[key] = true
		ignore = append(ignore, n)
	}
	c.Ignore = ignore
}

// NormalizeUI 归一化界面相关字段（数值范围、枚举）。
func (c *Config) NormalizeUI() {
	c.BgImage = strings.TrimSpace(c.BgImage)
	if c.BgOpacity < 0 {
		c.BgOpacity = 0
	}
	if c.BgOpacity > 1 {
		c.BgOpacity = 1
	}
	if c.BgOpacity == 0 {
		c.BgOpacity = 0.35
	}
	switch strings.ToLower(strings.TrimSpace(c.BgFit)) {
	case "cover":
		c.BgFit = "cover"
	case "stretch":
		c.BgFit = "stretch"
	case "tile":
		c.BgFit = "tile"
	default:
		c.BgFit = "contain"
	}
	switch strings.ToLower(strings.TrimSpace(c.BgScope)) {
	case "window":
		c.BgScope = "window"
	case "custom":
		c.BgScope = "custom"
	default:
		c.BgScope = "main"
	}
	c.BgRect = normalizeRect(c.BgRect)
	c.ThemeAccent = normalizeHex(c.ThemeAccent, "#e95420")
	c.ThemePage = normalizeHex(c.ThemePage, "#2c001e")
	c.ThemeBack = normalizeHex(c.ThemeBack, "")
	c.ThemeRecent = normalizeHex(c.ThemeRecent, "")

	// 字体：空值表示继承/跟随，字号 0 表示使用默认 13.5
	c.FontFamily = strings.TrimSpace(c.FontFamily)
	c.FontColor = normalizeHex(c.FontColor, "")
	if c.FontSize != 0 {
		if c.FontSize < 10 {
			c.FontSize = 10
		}
		if c.FontSize > 24 {
			c.FontSize = 24
		}
	}
	if c.FontParts == nil {
		c.FontParts = map[string]FontPart{}
	}
	for k, v := range c.FontParts {
		v.Family = strings.TrimSpace(v.Family)
		v.Color = normalizeHex(v.Color, "")
		if v.Size != 0 {
			if v.Size < 10 {
				v.Size = 10
			}
			if v.Size > 24 {
				v.Size = 24
			}
		}
		if v.Family == "" && v.Size == 0 && v.Color == "" {
			delete(c.FontParts, k)
			continue
		}
		c.FontParts[k] = v
	}
	switch strings.ToLower(strings.TrimSpace(c.CategoriesPosition)) {
	case "right":
		c.CategoriesPosition = "right"
	case "top":
		c.CategoriesPosition = "top"
	default:
		c.CategoriesPosition = "left"
	}
	if c.IconScale <= 0 {
		c.IconScale = 1
	}
	if c.IconScale < 0.6 {
		c.IconScale = 0.6
	}
	if c.IconScale > 2 {
		c.IconScale = 2
	}
	c.Hotkey = strings.TrimSpace(c.Hotkey)
	c.CandRuleFile = strings.TrimSpace(c.CandRuleFile)
	if c.SidebarWidth == 0 {
		c.SidebarWidth = 258
	}
	if c.SidebarWidth < 180 {
		c.SidebarWidth = 180
	}
	if c.SidebarWidth > 520 {
		c.SidebarWidth = 520
	}

	// 侧栏自定义项：去空白、去重、保证 map 可用
	seenFolders := map[string]bool{}
	folders := make([]string, 0, len(c.CustomFolders))
	for _, f := range c.CustomFolders {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		key := strings.ToLower(f)
		if seenFolders[key] {
			continue
		}
		seenFolders[key] = true
		folders = append(folders, f)
	}
	c.CustomFolders = folders

	if c.ItemAliases == nil {
		c.ItemAliases = map[string]string{}
	}
	if c.ItemIcons == nil {
		c.ItemIcons = map[string]string{}
	}
	if c.CategoryTargets == nil {
		c.CategoryTargets = map[string]string{}
	}
	seenHidden := map[string]bool{}
	hidden := make([]string, 0, len(c.HiddenCategories))
	for _, n := range c.HiddenCategories {
		n = strings.TrimSpace(n)
		if n == "" || seenHidden[strings.ToLower(n)] {
			continue
		}
		seenHidden[strings.ToLower(n)] = true
		hidden = append(hidden, n)
	}
	c.HiddenCategories = hidden

	if c.SearchCandidates == 0 {
		c.SearchCandidates = 8
	}
	if c.SearchCandidates < 3 {
		c.SearchCandidates = 3
	}
	if c.SearchCandidates > 30 {
		c.SearchCandidates = 30
	}
}

// normalizeRect 校验并归一化自定义背景区域 [x,y,w,h]。
func normalizeRect(v []float64) []float64 {
	full := []float64{0, 0, 1, 1}
	if len(v) != 4 {
		return full
	}
	out := make([]float64, 4)
	for i, n := range v {
		if n < 0 {
			n = 0
		}
		if n > 1 {
			n = 1
		}
		out[i] = n
	}
	if out[2] < 0.05 || out[3] < 0.05 {
		return full
	}
	if out[0]+out[2] > 1 {
		out[2] = 1 - out[0]
	}
	if out[1]+out[3] > 1 {
		out[3] = 1 - out[1]
	}
	return out
}

// normalizeHex 校验 #rrggbb 颜色；非法时返回 fallback。
func normalizeHex(v, fallback string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) != 7 || v[0] != '#' {
		return fallback
	}
	for _, ch := range v[1:] {
		if !(ch >= '0' && ch <= '9') && !(ch >= 'a' && ch <= 'f') {
			return fallback
		}
	}
	return v
}

// Validate 校验枚举与数值范围；非法配置直接报错，不静默修正。
func (c *Config) Validate() error {
	switch c.Conflict {
	case "index", "timestamp", "skip":
	default:
		return fmt.Errorf("配置 conflict = %q 非法，只能是 index / timestamp / skip", c.Conflict)
	}
	switch c.Verify {
	case "sha256", "size":
	default:
		return fmt.Errorf("配置 verify = %q 非法，只能是 sha256 / size", c.Verify)
	}
	if c.Concurrency < 1 || c.Concurrency > 32 {
		return fmt.Errorf("配置 concurrency = %d 非法，取值范围 1-32", c.Concurrency)
	}
	if c.ConfirmAboveMB < 0 {
		return fmt.Errorf("配置 confirm_above_mb = %d 非法，不能为负数", c.ConfirmAboveMB)
	}
	return nil
}

// Ignored 判断文件名是否在忽略列表中（不区分大小写）。
func (c *Config) Ignored(name string) bool {
	lower := strings.ToLower(name)
	for _, n := range c.Ignore {
		if strings.ToLower(n) == lower {
			return true
		}
	}
	return false
}

// IsSystem 判断是否为 Windows / macOS 的系统占位文件。
func (c *Config) IsSystem(name string) bool {
	switch strings.ToLower(name) {
	case "desktop.ini", "thumbs.db", ".ds_store":
		return true
	}
	return false
}

// Save 写出配置（覆盖）。
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "# dtidy 配置\n"+
		"# desktop 留空 = 自动探测；root_dir = \"home\" 表示分类都放在 <桌面>\\home 下\n\n"); err != nil {
		return err
	}
	return toml.NewEncoder(f).Encode(c)
}

// RenderDefault 返回默认配置的文本内容（用于 config init 输出提示）。
func RenderDefault() (string, error) {
	cfg := Default()
	p, err := Path()
	if err != nil {
		return "", err
	}
	if err := cfg.Save(p); err != nil {
		return "", err
	}
	return p, nil
}
