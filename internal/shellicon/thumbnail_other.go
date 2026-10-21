//go:build !windows

package shellicon

// IsImage 在非 Windows 平台不做缩略图（保留接口）。
func IsImage(path string) bool { return false }

// Thumbnail 在非 Windows 平台返回空字符串。
func Thumbnail(path string) string { return "" }
