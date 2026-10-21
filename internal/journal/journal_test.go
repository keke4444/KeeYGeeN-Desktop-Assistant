package journal

import (
	"os"
	"path/filepath"
	"testing"

	"dtidy/internal/fsops"
)

func TestUndoRestoresFiles(t *testing.T) {
	desktop := t.TempDir()
	categoryDir := filepath.Join(desktop, "工具")
	if err := os.MkdirAll(categoryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{"a.png", "b.txt"}
	for _, n := range names {
		p := filepath.Join(desktop, n)
		if err := os.WriteFile(p, []byte("content-"+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	j := OpenAt(filepath.Join(t.TempDir(), "journal.jsonl"))
	batch := "20261020-120000-abcd"

	for _, n := range names {
		res := fsops.MoveFile(filepath.Join(desktop, n), categoryDir, fsops.Options{Verify: "sha256", Conflict: "index"})
		if !res.Ok {
			t.Fatalf("移动 %s 失败: %s", n, res.Err)
		}
		if err := j.Append(Record{Batch: batch, Action: "move", Src: res.Src, Dst: res.Dst, Size: res.Size, SHA256: res.SHA256, Verify: "sha256", OK: true}); err != nil {
			t.Fatal(err)
		}
	}

	results, used, err := Undo(j, "", "sha256")
	if err != nil {
		t.Fatalf("撤销失败: %v", err)
	}
	if used != batch {
		t.Fatalf("期望批次 %s，得到 %s", batch, used)
	}
	if len(results) != 2 {
		t.Fatalf("期望 2 条结果，得到 %d", len(results))
	}
	for _, n := range names {
		body, err := os.ReadFile(filepath.Join(desktop, n))
		if err != nil {
			t.Fatalf("%s 未回到桌面: %v", n, err)
		}
		if string(body) != "content-"+n {
			t.Fatalf("%s 内容不一致: %q", n, body)
		}
		if _, err := os.Stat(filepath.Join(categoryDir, n)); !os.IsNotExist(err) {
			t.Fatalf("%s 的副本应已移除", n)
		}
	}
}

func TestUndoSkipsWhenSourceExists(t *testing.T) {
	desktop := t.TempDir()
	categoryDir := filepath.Join(desktop, "工具")
	if err := os.MkdirAll(categoryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(desktop, "a.png")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := OpenAt(filepath.Join(t.TempDir(), "journal.jsonl"))

	res := fsops.MoveFile(src, categoryDir, fsops.Options{Verify: "size", Conflict: "index"})
	if !res.Ok {
		t.Fatalf("移动失败: %s", res.Err)
	}
	if err := j.Append(Record{Batch: "b1", Action: "move", Src: res.Src, Dst: res.Dst, OK: true}); err != nil {
		t.Fatal(err)
	}
	// 撤销前在源位置重新放一个文件
	if err := os.WriteFile(src, []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, _, err := Undo(j, "b1", "size")
	if err != nil {
		t.Fatalf("撤销失败: %v", err)
	}
	if len(results) != 1 || !results[0].Skipped {
		t.Fatalf("源位置已有文件时应跳过，得到 %+v", results)
	}
	body, _ := os.ReadFile(src)
	if string(body) != "occupied" {
		t.Fatalf("源文件被覆盖: %q", body)
	}
}
