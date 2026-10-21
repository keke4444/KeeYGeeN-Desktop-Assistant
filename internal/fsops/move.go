package fsops

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options 控制单次移动行为。
type Options struct {
	Verify   string // sha256 | size
	Conflict string // index | timestamp | skip
}

// MoveResult 描述单个文件的移动结果。
type MoveResult struct {
	Src     string
	Dst     string
	Size    int64
	SHA256  string
	Verify  string
	Ok      bool
	Skipped bool
	Err     string
}

// DeleteSourceError 表示目标副本已就位，但删除源文件失败（严禁重试删除）。
type DeleteSourceError struct{ Err error }

func (e *DeleteSourceError) Error() string {
	return "目标已就位，但源文件删除失败：" + e.Err.Error()
}

// TestHook 仅用于测试注入故障；生产环境为 nil。
// stage 取值为 "after-copy" / "before-delete"。
var TestHook func(stage, path string) error

func hook(stage, path string) error {
	if TestHook == nil {
		return nil
	}
	return TestHook(stage, path)
}

// MoveFile 把 src 移动到 targetDir 下，遵循「复制 → 校验 → 重命名 → 删源」。
func MoveFile(src, targetDir string, opt Options) MoveResult {
	res := MoveResult{Src: src, Verify: opt.Verify}

	info, err := os.Stat(src)
	if err != nil {
		res.Err = "源文件不可访问：" + err.Error()
		return res
	}
	if !info.Mode().IsRegular() {
		res.Skipped = true
		res.Err = "不是普通文件，已跳过"
		return res
	}
	res.Size = info.Size()

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		res.Err = "创建目标目录失败：" + err.Error()
		return res
	}

	dst, skipReason, err := resolveConflict(targetDir, filepath.Base(src), opt.Conflict)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	if skipReason != "" {
		res.Skipped = true
		res.Err = skipReason
		return res
	}
	res.Dst = dst

	sha, err := moveToExact(src, dst, opt.Verify)
	res.SHA256 = sha
	if err != nil {
		var delErr *DeleteSourceError
		if errors.As(err, &delErr) {
			// 目标已就位，源仍在：按「成功但需人工处理」上报，绝不重试删除
			res.Ok = true
			res.Err = err.Error()
			return res
		}
		res.Err = err.Error()
		return res
	}
	res.Ok = true
	return res
}

// moveToExact 把 src 精确移动到 dst（dst 必须不存在）。
// 顺序：复制到同目录临时文件 → 校验 → 原子重命名 → 删除源文件。
func moveToExact(src, dst, verify string) (string, error) {
	if _, err := os.Lstat(dst); err == nil {
		return "", fmt.Errorf("目标已存在：%s", dst)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(dst)

	tmp, err := os.CreateTemp(dir, ".dtidy-tmp-*")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败：%w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpPath)
	}

	in, err := os.Open(src)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("打开源文件失败：%w", err)
	}
	buf := make([]byte, 64*1024)
	if _, err := io.CopyBuffer(tmp, in, buf); err != nil {
		in.Close()
		cleanup()
		return "", fmt.Errorf("复制失败：%w", err)
	}
	in.Close()
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("写入目标失败：%w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("关闭目标文件失败：%w", err)
	}

	if err := os.Chmod(tmpPath, srcInfo.Mode().Perm()); err != nil {
		// Windows 上权限位意义有限，失败不致命
		_ = err
	}
	_ = os.Chtimes(tmpPath, time.Now(), srcInfo.ModTime())

	if err := hook("after-copy", tmpPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("校验失败：%w", err)
	}

	sha, err := verifyCopy(src, tmpPath, verify)
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("重命名到目标失败：%w", err)
	}

	if err := hook("before-delete", src); err != nil {
		return sha, &DeleteSourceError{Err: err}
	}
	if err := os.Remove(src); err != nil {
		return sha, &DeleteSourceError{Err: err}
	}
	return sha, nil
}

// MoveToExact 供撤销使用：把 src 精确移动到 dst（dst 必须不存在），返回 SHA256。
func MoveToExact(src, dst, verify string) (string, error) {
	return moveToExact(src, dst, verify)
}

// verifyCopy 比对两侧文件，返回目标文件摘要（size 模式返回空）。
func verifyCopy(src, dst, mode string) (string, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return "", err
	}
	if srcInfo.Size() != dstInfo.Size() {
		return "", fmt.Errorf("校验失败：大小不一致（源 %d，目标 %d）", srcInfo.Size(), dstInfo.Size())
	}
	if mode == "size" {
		return "", nil
	}
	srcSum, err := hashFile(src)
	if err != nil {
		return "", fmt.Errorf("计算源文件摘要失败：%w", err)
	}
	dstSum, err := hashFile(dst)
	if err != nil {
		return "", fmt.Errorf("计算目标文件摘要失败：%w", err)
	}
	if srcSum != dstSum {
		return "", fmt.Errorf("校验失败：SHA256 不一致")
	}
	return dstSum, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 64*1024)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// resolveConflict 返回最终目标路径；skipReason 非空表示应跳过该文件。
func resolveConflict(dir, name, mode string) (string, string, error) {
	candidate := filepath.Join(dir, name)
	if !pathExists(candidate) {
		return candidate, "", nil
	}
	switch mode {
	case "skip":
		return "", "目标已存在同名文件，已跳过", nil
	case "timestamp":
		stamp := time.Now().Format("20060102-150405")
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		for i := 0; i < 10000; i++ {
			suffix := "_" + stamp
			if i > 0 {
				suffix = fmt.Sprintf("_%s-%d", stamp, i+1)
			}
			c := filepath.Join(dir, base+suffix+ext)
			if !pathExists(c) {
				return c, "", nil
			}
		}
		return "", "", fmt.Errorf("无法为 %q 生成不冲突的文件名", name)
	default: // index
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		for i := 2; i < 10000; i++ {
			c := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
			if !pathExists(c) {
				return c, "", nil
			}
		}
		return "", "", fmt.Errorf("无法为 %q 生成不冲突的文件名", name)
	}
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
