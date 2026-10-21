//go:build windows

package shellicon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIconForCommonExtensions(t *testing.T) {
	dir := t.TempDir()
	samples := map[string]string{
		"a.txt":   "hello",
		"b.png":   "not-a-real-png",
		"c.zip":   "zip",
		"d.exe":   "MZ",
		"e.lnk":   "lnk",
		"README":  "no ext",
	}
	for name, body := range samples {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		url := IconFor(p, false)
		if url == "" {
			t.Errorf("%s 未取到图标", name)
			continue
		}
		if !strings.HasPrefix(url, "data:image/png;base64,") {
			t.Errorf("%s 图标格式不正确: %.40s", name, url)
		}
		if len(url) < 200 {
			t.Errorf("%s 图标数据过小: %d 字节", name, len(url))
		}
	}
}

func TestFolderIcon(t *testing.T) {
	url := FolderIcon()
	if url == "" {
		t.Fatal("文件夹图标为空")
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("文件夹图标格式不正确: %.40s", url)
	}
}

func TestIconCacheByExtension(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "one.txt")
	b := filepath.Join(dir, "two.txt")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	first := IconFor(a, false)
	second := IconFor(b, false)
	if first == "" || second == "" {
		t.Fatal("图标为空")
	}
	if first != second {
		t.Fatal("同扩展名应命中同一缓存")
	}
}
