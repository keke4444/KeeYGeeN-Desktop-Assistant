package shellicon

// PreviewFor 优先返回图片缩略图；非图片或解码失败时回退到系统文件图标。
// 独立文件、无平台构建标签：两个平台的具体实现分别由 icons_*.go 与 thumbnail_*.go 提供。
func PreviewFor(path string, isDir bool) string {
	if !isDir && IsImage(path) {
		if thumb := Thumbnail(path); thumb != "" {
			return thumb
		}
	}
	return IconFor(path, isDir)
}
