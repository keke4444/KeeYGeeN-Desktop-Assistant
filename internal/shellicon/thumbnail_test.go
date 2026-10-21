//go:build windows

package shellicon

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTestPNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 160, A: 255})
		}
	}
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestThumbnailForPNG(t *testing.T) {
	dir := t.TempDir()
	p := makeTestPNG(t, dir, "photo.png", 400, 300)

	if !IsImage(p) {
		t.Fatal("png 应被识别为图片")
	}
	url := Thumbnail(p)
	if url == "" {
		t.Fatal("缩略图为空")
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("缩略图格式不正确: %.40s", url)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/png;base64,"))
	if err != nil {
		t.Fatalf("缩略图解码失败: %v", err)
	}
	thumb, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("缩略图不是有效 PNG: %v", err)
	}
	if b := thumb.Bounds(); b.Dx() != 160 || b.Dy() != 120 {
		t.Fatalf("期望缩略图 160x120，实际 %dx%d", b.Dx(), b.Dy())
	}

	// 命中缓存
	if Thumbnail(p) != url {
		t.Fatal("同一文件应命中缩略图缓存")
	}

	// 预览接口应返回缩略图而不是类型图标
	if PreviewFor(p, false) != url {
		t.Fatal("PreviewFor 应返回图片缩略图")
	}
}

func TestThumbnailFallsBackForText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsImage(p) {
		t.Fatal("txt 不应被识别为图片")
	}
	if Thumbnail(p) != "" {
		t.Fatal("文本文件不应生成缩略图")
	}
	if PreviewFor(p, false) == "" {
		t.Fatal("文本文件应回退到系统图标")
	}
}

func TestFitSquareKeepsAspect(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 200))
	out := fitSquare(src, 80)
	if out == nil {
		t.Fatal("缩放结果为空")
	}
	if b := out.Bounds(); b.Dx() != 80 || b.Dy() != 40 {
		t.Fatalf("期望 80x40，实际 %dx%d", b.Dx(), b.Dy())
	}
}
