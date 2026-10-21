package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"dtidy/internal/category"
	"dtidy/internal/config"
	"dtidy/internal/fsops"
)

func setup(t *testing.T) (Model, string) {
	t.Helper()
	desktop := t.TempDir()
	for _, n := range []string{"a.png", "b.txt", "README"} {
		if err := os.WriteFile(filepath.Join(desktop, n), []byte("x-"+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Default()
	if err := os.MkdirAll(filepath.Join(desktop, cfg.RootDir, "工具"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := category.Build(filepath.Join(desktop, cfg.RootDir), cfg.DiscoverCategories, cfg.ExtraCategories, cfg.ExcludeCategories)
	if err != nil {
		t.Fatal(err)
	}
	return New(desktop, &cfg, reg, nil, "test-batch"), desktop
}

func TestViewRendersInSeveralStates(t *testing.T) {
	m, _ := setup(t)

	sizes := [][2]int{{100, 30}, {80, 24}, {60, 15}, {40, 10}}
	for _, s := range sizes {
		m.width, m.height = s[0], s[1]
		if out := m.View(); out == "" {
			t.Fatalf("尺寸 %dx%d 渲染为空", s[0], s[1])
		}
	}

	m.width, m.height = 100, 30
	m.st = stateConfirm
	m.pending = []item{{entry: m.files[0], cat: "工具"}}
	_ = m.View()
	m.st = statePick
	_ = m.View()
	m.st = stateHelp
	_ = m.View()
	m.st = stateProgress
	m.total = 1
	m.done = 1
	m.current = "a.png"
	m.results = []fsops.MoveResult{{Src: "a.png", Ok: true}}
	_ = m.View()
	m.st = stateReport
	_ = m.View()

	empty := t.TempDir()
	emptyReg, _ := category.Build(empty, true, nil, nil)
	cfg := config.Default()
	m2 := New(empty, &cfg, emptyReg, nil, "b")
	m2.width, m2.height = 80, 24
	if out := m2.View(); out == "" {
		t.Fatal("空桌面渲染为空")
	}
}

func TestMarkAndMoveFlow(t *testing.T) {
	m, desktop := setup(t)

	if m.cursor != 0 || m.files[0].Name != "a.png" {
		t.Fatalf("期望光标在 a.png，实际 %q", m.files[0].Name)
	}

	// Space 标记
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	if len(m.marked) != 1 {
		t.Fatalf("期望标记 1 项，实际 %d", len(m.marked))
	}

	// Enter → 确认浮层
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.st != stateConfirm {
		t.Fatalf("期望进入确认态，实际 %v", m.st)
	}
	if len(m.pending) != 1 || m.pending[0].cat != "工具" {
		t.Fatalf("确认项不正确: %+v", m.pending)
	}

	// y → 执行
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)
	if m.st != stateProgress {
		t.Fatalf("期望进入进度态，实际 %v", m.st)
	}

	drain(t, &m, cmd)

	if m.st != stateReport {
		t.Fatalf("期望进入报告态，实际 %v", m.st)
	}
	if _, err := os.Stat(filepath.Join(desktop, "home", "工具", "a.png")); err != nil {
		t.Fatalf("目标文件不存在: %v", err)
	}
	if _, err := os.Stat(filepath.Join(desktop, "a.png")); !os.IsNotExist(err) {
		t.Fatalf("源文件应已被移除")
	}

	// 回到列表
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.st != stateList {
		t.Fatalf("期望回到列表，实际 %v", m.st)
	}
	if len(m.files) != 2 {
		t.Fatalf("重新扫描后期望 2 个文件，实际 %d", len(m.files))
	}
}

// drain 执行命令队列直到进度结束。
func drain(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; steps < 500 && len(queue) > 0; steps++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if msg == nil {
			continue
		}
		switch v := msg.(type) {
		case tea.BatchMsg:
			queue = append(queue, v...)
		case moveFinished:
			next, _ := m.Update(v)
			*m = next.(Model)
			return
		default:
			next, nc := m.Update(v)
			*m = next.(Model)
			if nc != nil {
				queue = append(queue, nc)
			}
		}
	}
	t.Fatalf("进度未在预期步数内结束")
}

func TestTabChangesCategory(t *testing.T) {
	m, desktop := setup(t)
	if err := os.MkdirAll(filepath.Join(desktop, "home", "编程"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 重新扫描以发现新分类
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)

	if len(m.reg.Categories) != 2 {
		t.Fatalf("期望 2 个分类，实际 %d: %v", len(m.reg.Categories), m.reg.Names())
	}
	first := m.currentCategory()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.currentCategory() == first {
		t.Fatalf("Tab 未切换分类")
	}
}

func TestEnterWithoutCategoryShowsHint(t *testing.T) {
	desktop := t.TempDir()
	if err := os.WriteFile(filepath.Join(desktop, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	reg, _ := category.Build(desktop, true, nil, nil)
	m := New(desktop, &cfg, reg, nil, "b")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.st != stateList {
		t.Fatalf("无分类时不应进入确认态，实际 %v", m.st)
	}
	if m.status == "" {
		t.Fatalf("应给出提示文案")
	}
}
