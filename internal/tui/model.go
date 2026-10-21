package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"dtidy/internal/category"
	"dtidy/internal/config"
	"dtidy/internal/fsops"
	"dtidy/internal/journal"
)

type state int

const (
	stateList state = iota
	stateConfirm
	stateProgress
	stateReport
	stateHelp
	statePick
)

// item 表示一次待移动：文件 + 目标分类名。
type item struct {
	entry fsops.Entry
	cat   string
}

type moveProgress struct {
	done  int
	total int
	res   fsops.MoveResult
	name  string
}

type moveFinished struct{}

type spinnerMsg struct{}

// Model 是 TUI 的全部状态。
type Model struct {
	desktop string
	cfg     *config.Config
	reg     *category.Registry
	j       *journal.Journal
	batch   string

	files    []fsops.Entry
	cursor   int
	offset   int
	catIdx   int
	marked   map[string]bool
	override map[string]string

	st     state
	width  int
	height int
	status string
	spin   int
	err    error

	pending     []item
	confirmCat  string
	pickIdx     int
	results     []fsops.MoveResult
	showDetails bool
	done        int
	total       int
	current     string
	cancelled   bool
	ch          chan tea.Msg
	cancel      *atomic.Bool
}

// New 创建模型并完成首次扫描。
func New(desktop string, cfg *config.Config, reg *category.Registry, j *journal.Journal, batch string) Model {
	m := Model{
		desktop:  desktop,
		cfg:      cfg,
		reg:      reg,
		j:        j,
		batch:    batch,
		marked:   map[string]bool{},
		override: map[string]string{},
		width:    100,
		height:   30,
	}
	m.reload()
	return m
}

func (m *Model) reload() {
	res, err := fsops.Scan(m.desktop, m.cfg.ShowHidden, m.cfg.Ignore)
	if err != nil {
		m.err = fmt.Errorf("扫描桌面失败：%w", err)
		return
	}
	m.files = res.Files
	if m.cursor >= len(m.files) {
		m.cursor = len(m.files) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
}

// rescan 重新发现分类并重新扫描文件（删除的标记与覆盖在重扫后失效）。
func (m *Model) rescan() {
	root := m.desktop
	if m.cfg.RootDir != "" {
		root = filepath.Join(m.desktop, m.cfg.RootDir)
	}
	reg, err := category.Build(root, m.cfg.DiscoverCategories,
		m.cfg.ExtraCategories, m.cfg.ExcludeCategories)
	if err != nil {
		m.err = err
		return
	}
	m.reg = reg
	if m.catIdx >= len(reg.Categories) {
		m.catIdx = len(reg.Categories) - 1
	}
	if m.catIdx < 0 {
		m.catIdx = 0
	}
	m.marked = map[string]bool{}
	m.override = map[string]string{}
	m.reload()
}

func (m Model) showDetailLines() bool { return m.height >= 18 }

// vw 返回安全渲染宽度：比终端宽度少 1 列，避免恰好撑满触发自动换行。
func (m Model) vw() int {
	w := m.width
	if w <= 0 {
		w = 100
	}
	w--
	if w < 20 {
		w = 20
	}
	return w
}

func (m Model) listHeight() int {
	h := m.height - 4
	if m.showDetailLines() {
		h -= 2
	}
	if h < 3 {
		h = 3
	}
	return h
}

func (m *Model) ensureVisible() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) currentCategory() string {
	if len(m.reg.Categories) == 0 {
		return ""
	}
	if m.catIdx < 0 || m.catIdx >= len(m.reg.Categories) {
		return m.reg.Categories[0].Name
	}
	return m.reg.Categories[m.catIdx].Name
}

// targets 计算本次要移动的文件（标记优先，否则光标项）。
func (m *Model) targets() []item {
	var chosen []fsops.Entry
	if len(m.marked) > 0 {
		for _, f := range m.files {
			if m.marked[f.Path] {
				chosen = append(chosen, f)
			}
		}
	} else if len(m.files) > 0 && m.cursor < len(m.files) {
		chosen = append(chosen, m.files[m.cursor])
	}

	base := m.currentCategory()
	var out []item
	for _, f := range chosen {
		cat := base
		if o, ok := m.override[f.Path]; ok {
			cat = o
		}
		if cat == "" {
			continue
		}
		out = append(out, item{entry: f, cat: cat})
	}
	return out
}

func (m *Model) toggleMark() {
	if len(m.files) == 0 || m.cursor >= len(m.files) {
		return
	}
	p := m.files[m.cursor].Path
	if m.marked[p] {
		delete(m.marked, p)
	} else {
		m.marked[p] = true
	}
}

func (m *Model) toggleAll() {
	if len(m.marked) == len(m.files) {
		m.marked = map[string]bool{}
		return
	}
	for _, f := range m.files {
		m.marked[f.Path] = true
	}
}

func (m *Model) beginMove() tea.Cmd {
	items := m.pending
	total := len(items)
	m.st = stateProgress
	m.results = nil
	m.done = 0
	m.total = total
	m.current = ""
	m.cancelled = false

	cancel := &atomic.Bool{}
	m.cancel = cancel
	ch := make(chan tea.Msg, total+2)
	m.ch = ch

	desktop := m.desktop
	root := m.cfg.RootDir
	opt := fsops.Options{Verify: m.cfg.Verify, Conflict: m.cfg.Conflict}
	j := m.j
	batch := m.batch

	go func() {
		for i, it := range items {
			if cancel.Load() {
				break
			}
			targetDir := filepath.Join(desktop, root, it.cat)
			res := fsops.MoveFile(it.entry.Path, targetDir, opt)
			rec := journal.Record{
				Batch:  batch,
				Action: "move",
				Src:    res.Src,
				Dst:    res.Dst,
				Size:   res.Size,
				SHA256: res.SHA256,
				Verify: res.Verify,
				OK:     res.Ok,
				Err:    res.Err,
			}
			if j != nil {
				_ = j.Append(rec)
			}
			ch <- moveProgress{done: i + 1, total: total, res: res, name: it.entry.Name}
		}
		ch <- moveFinished{}
	}()

	return tea.Batch(waitFor(ch), tickSpinner())
}

func waitFor(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return moveFinished{}
		}
		return msg
	}
}

func tickSpinner() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinnerMsg{} })
}

// ---- 展示辅助 ----

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

func padTo(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > w-1 {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String() + "…"
}

func elideMiddle(s string, w int) string {
	if w <= 1 || lipgloss.Width(s) <= w {
		return s
	}
	runes := []rune(s)
	// 保留首尾：前面留多一点，尾部保留末级目录
	head := w / 3
	tail := w - head - 1
	if head < 1 || tail < 1 {
		return truncate(s, w)
	}
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}
