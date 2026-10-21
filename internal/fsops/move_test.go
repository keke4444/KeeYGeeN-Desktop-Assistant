package fsops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	return p
}

func countTmpFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".dtidy-tmp-") {
			n++
		}
	}
	return n
}

func TestMoveFileSuccess(t *testing.T) {
	desktop := t.TempDir()
	target := filepath.Join(desktop, "工具")
	src := writeFile(t, desktop, "ida64.exe.lnk", "shortcut-content")

	res := MoveFile(src, target, Options{Verify: "sha256", Conflict: "index"})
	if !res.Ok || res.Err != "" {
		t.Fatalf("期望成功，得到 Ok=%v Err=%q", res.Ok, res.Err)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("源文件应已不存在: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(target, "ida64.exe.lnk"))
	if err != nil {
		t.Fatalf("目标文件不存在: %v", err)
	}
	if string(got) != "shortcut-content" {
		t.Fatalf("内容不一致: %q", got)
	}
	if n := countTmpFiles(t, target); n != 0 {
		t.Fatalf("不应残留临时文件，实际 %d 个", n)
	}
}

func TestMoveFileConflictIndex(t *testing.T) {
	desktop := t.TempDir()
	target := filepath.Join(desktop, "工具")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, target, "a.png", "old")
	src := writeFile(t, desktop, "a.png", "new")

	res := MoveFile(src, target, Options{Verify: "size", Conflict: "index"})
	if !res.Ok {
		t.Fatalf("期望成功，得到 Err=%q", res.Err)
	}
	if filepath.Base(res.Dst) != "a (2).png" {
		t.Fatalf("期望 a (2).png，得到 %s", filepath.Base(res.Dst))
	}
	old, _ := os.ReadFile(filepath.Join(target, "a.png"))
	if string(old) != "old" {
		t.Fatalf("已存在文件被覆盖: %q", old)
	}
}

func TestMoveFileConflictSkip(t *testing.T) {
	desktop := t.TempDir()
	target := filepath.Join(desktop, "工具")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, target, "a.png", "old")
	src := writeFile(t, desktop, "a.png", "new")

	res := MoveFile(src, target, Options{Verify: "sha256", Conflict: "skip"})
	if !res.Skipped || res.Ok {
		t.Fatalf("期望跳过，得到 Ok=%v Skipped=%v Err=%q", res.Ok, res.Skipped, res.Err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("跳过时源文件必须保持原样: %v", err)
	}
}

func TestMoveFileVerifyFailureKeepsSource(t *testing.T) {
	desktop := t.TempDir()
	target := filepath.Join(desktop, "工具")
	src := writeFile(t, desktop, "a.png", "data")

	TestHook = func(stage, path string) error {
		if stage == "after-copy" {
			return errors.New("注入的校验故障")
		}
		return nil
	}
	defer func() { TestHook = nil }()

	res := MoveFile(src, target, Options{Verify: "sha256", Conflict: "index"})
	if res.Ok {
		t.Fatalf("注入故障后不应成功")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("失败时源文件必须存在: %v", err)
	}
	if n := countTmpFiles(t, target); n != 0 {
		t.Fatalf("失败时不应残留临时文件，实际 %d 个", n)
	}
}

func TestMoveFileDeleteSourceFailure(t *testing.T) {
	desktop := t.TempDir()
	target := filepath.Join(desktop, "工具")
	src := writeFile(t, desktop, "a.png", "data")

	TestHook = func(stage, path string) error {
		if stage == "before-delete" {
			return errors.New("注入的删除故障")
		}
		return nil
	}
	defer func() { TestHook = nil }()

	res := MoveFile(src, target, Options{Verify: "size", Conflict: "index"})
	if !res.Ok {
		t.Fatalf("目标已就位时应标记为成功，得到 Err=%q", res.Err)
	}
	if res.Err == "" {
		t.Fatalf("应明确提示源文件未删除")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("原文件应仍存在（保留双份）: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "a.png")); err != nil {
		t.Fatalf("目标副本应存在: %v", err)
	}
}

func TestMoveToExactRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "a.txt", "src")
	dst := writeFile(t, dir, "b.txt", "dst")

	if _, err := MoveToExact(src, dst, "size"); err == nil {
		t.Fatalf("目标已存在时必须报错，不能覆盖")
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "dst" {
		t.Fatalf("目标文件被覆盖: %q", got)
	}
}
