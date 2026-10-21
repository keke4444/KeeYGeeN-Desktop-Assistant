package main

import (
	"os"
	"path/filepath"
	"testing"

	"dtidy/internal/config"
	"dtidy/internal/journal"
)

func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	desktop := t.TempDir()
	root := filepath.Join(desktop, "home")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.RootDir = "home"
	return &App{
		cfg:     &cfg,
		desktop: desktop,
		root:    root,
		j:       journal.OpenAt(filepath.Join(t.TempDir(), "journal.jsonl")),
		batch:   "test-batch",
	}, desktop
}

func TestCreateCategoryAndMove(t *testing.T) {
	a, desktop := newTestApp(t)
	src := filepath.Join(desktop, "a.png")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := a.CreateCategory("工具"); err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(desktop, "home", "工具")); err != nil || !fi.IsDir() {
		t.Fatalf("分类目录未创建: %v", err)
	}

	st := a.GetState()
	if len(st.Categories) != 1 || st.Categories[0].Name != "工具" {
		t.Fatalf("分类列表不正确: %+v", st.Categories)
	}

	report, err := a.MoveTo([]string{src}, filepath.Join(desktop, "home", "工具"))
	if err != nil {
		t.Fatalf("移动失败: %v", err)
	}
	if report.Ok != 1 || report.Failed != 0 {
		t.Fatalf("移动结果不正确: %+v", report)
	}
	if _, err := os.ReadFile(filepath.Join(desktop, "home", "工具", "a.png")); err != nil {
		t.Fatalf("目标文件不存在: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("源文件应已移除")
	}

	listing, err := a.ListDir(filepath.Join(desktop, "home", "工具"))
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "a.png" {
		t.Fatalf("分类目录列表不正确: %+v", listing.Entries)
	}

	// 移回桌面
	report, err = a.MoveTo([]string{filepath.Join(desktop, "home", "工具", "a.png")}, desktop)
	if err != nil || report.Ok != 1 {
		t.Fatalf("移回桌面失败: %v %+v", err, report)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("文件未回到桌面: %v", err)
	}
}

func TestMoveRejectsOutsideDestination(t *testing.T) {
	a, desktop := newTestApp(t)
	src := filepath.Join(desktop, "a.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.MoveTo([]string{src}, filepath.Dir(desktop)); err == nil {
		t.Fatalf("目标在桌面和 home 之外时必须拒绝")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("拒绝时源文件必须保持原样: %v", err)
	}
}

func TestMoveRejectsDirectory(t *testing.T) {
	a, desktop := newTestApp(t)
	dir := filepath.Join(desktop, "某个文件夹")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCategory("工具"); err != nil {
		t.Fatal(err)
	}
	report, err := a.MoveTo([]string{dir}, filepath.Join(desktop, "home", "工具"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed != 1 {
		t.Fatalf("文件夹不应被移动: %+v", report)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("文件夹必须保持原样")
	}
}

func TestUndoAfterMove(t *testing.T) {
	a, desktop := newTestApp(t)
	src := filepath.Join(desktop, "b.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCategory("文档"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.MoveTo([]string{src}, filepath.Join(desktop, "home", "文档")); err != nil {
		t.Fatal(err)
	}
	rep, err := a.Undo()
	if err != nil {
		t.Fatalf("撤销失败: %v", err)
	}
	if rep.Ok != 1 {
		t.Fatalf("撤销结果不正确: %+v", rep)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("文件未回到桌面: %v", err)
	}
}

func TestCreateCategoryRejectsIllegalName(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.CreateCategory("re/pwn"); err == nil {
		t.Fatalf("非法分类名必须报错")
	}
}
