//go:build windows

package shellicon

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
)

const (
	thumbEdge      = 160       // 生成缩略图的长边像素
	maxDecodeBytes = 48 << 20 // 超过 48 MB 的图片不解码，避免卡顿
)

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".webp": true, ".ico": true, ".tif": true, ".tiff": true,
}

var thumbCache sync.Map // path|size|modtime -> data URL

// IsImage 判断是否为可生成缩略图的图片文件。
func IsImage(path string) bool {
	return imageExts[strings.ToLower(filepath.Ext(path))]
}

// Thumbnail 生成图片缩略图的 data URL；失败返回空字符串。
func Thumbnail(path string) string {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() > maxDecodeBytes {
		return ""
	}
	key := path + "|" + fi.ModTime().String() + "|" + itoa(fi.Size())
	if v, ok := thumbCache.Load(key); ok {
		return v.(string)
	}

	url := buildThumbnail(path)
	thumbCache.Store(key, url)
	return url
}

func buildThumbnail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var src image.Image
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".bmp" {
		src, err = bmp.Decode(f)
	} else {
		src, _, err = image.Decode(f)
	}
	if err != nil || src == nil {
		return ""
	}

	thumb := fitSquare(src, thumbEdge)
	if thumb == nil {
		return ""
	}
	var out bytes.Buffer
	if err := png.Encode(&out, thumb); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out.Bytes())
}

// fitSquare 等比缩放到长边不超过 edge，保持原始宽高比。
func fitSquare(src image.Image, edge int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	nw, nh := w, h
	if w > edge || h > edge {
		if w >= h {
			nw = edge
			nh = h * edge / w
		} else {
			nh = edge
			nw = w * edge / h
		}
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
