package journal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dtidy/internal/config"
	"dtidy/internal/fsops"
)

// Record 是 journal.jsonl 的一行。
type Record struct {
	TS     time.Time `json:"ts"`
	Batch  string    `json:"batch"`
	Action string    `json:"action"` // move | undo
	Src    string    `json:"src"`
	Dst    string    `json:"dst"`
	Size   int64     `json:"size"`
	SHA256 string    `json:"sha256,omitempty"`
	Verify string    `json:"verify,omitempty"`
	OK     bool      `json:"ok"`
	Err    string    `json:"err,omitempty"`
}

// Journal 追加写入 JSONL。
type Journal struct {
	path string
}

// Open 打开（或创建）日志文件。
func Open() (*Journal, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Journal{path: filepath.Join(dir, "journal.jsonl")}, nil
}

// OpenAt 使用指定路径的日志文件（测试用）。
func OpenAt(path string) *Journal { return &Journal{path: path} }

// Path 返回日志文件绝对路径。
func (j *Journal) Path() string { return j.path }

// Append 追加一条记录并落盘。
func (j *Journal) Append(r Record) error {
	if r.TS.IsZero() {
		r.TS = time.Now()
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// All 读取全部记录。
func (j *Journal) All() ([]Record, error) {
	f, err := os.Open(j.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue // 跳过损坏行，不让日志影响主流程
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// LastMoveBatch 返回最近一个成功的移动批次 ID。
func (j *Journal) LastMoveBatch() (string, error) {
	records, err := j.All()
	if err != nil {
		return "", err
	}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Action == "move" && records[i].OK {
			return records[i].Batch, nil
		}
	}
	return "", fmt.Errorf("没有可撤销的批次")
}

// UndoResult 描述单条撤销结果。
type UndoResult struct {
	Dst     string
	Src     string
	Ok      bool
	Skipped bool
	Reason  string
}

// Undo 撤销指定批次；batch 为空时撤销最近一个成功批次。
func Undo(j *Journal, batch, verify string) ([]UndoResult, string, error) {
	records, err := j.All()
	if err != nil {
		return nil, "", err
	}
	if batch == "" {
		for i := len(records) - 1; i >= 0; i-- {
			if records[i].Action == "move" && records[i].OK {
				batch = records[i].Batch
				break
			}
		}
		if batch == "" {
			return nil, "", fmt.Errorf("没有可撤销的批次")
		}
	}

	var pending []Record
	for _, r := range records {
		if r.Action == "move" && r.OK && r.Batch == batch {
			pending = append(pending, r)
		}
	}
	if len(pending) == 0 {
		return nil, batch, fmt.Errorf("批次 %s 没有可撤销的记录", batch)
	}

	var results []UndoResult
	for i := len(pending) - 1; i >= 0; i-- {
		r := pending[i]
		ur := UndoResult{Dst: r.Dst, Src: r.Src}

		if _, statErr := os.Stat(r.Dst); statErr != nil {
			ur.Skipped = true
			ur.Reason = "目标不存在，可能已被手动移动"
			results = append(results, ur)
			continue
		}
		if _, statErr := os.Stat(r.Src); statErr == nil {
			ur.Skipped = true
			ur.Reason = "源位置已有同名文件，为避免覆盖已跳过"
			results = append(results, ur)
			continue
		}

		sha, moveErr := fsops.MoveToExact(r.Dst, r.Src, verify)
		ur.Ok = moveErr == nil
		if moveErr != nil {
			ur.Reason = moveErr.Error()
		}
		results = append(results, ur)

		rec := Record{
			Batch:  batch,
			Action: "undo",
			Src:    r.Dst,
			Dst:    r.Src,
			Size:   r.Size,
			SHA256: sha,
			Verify: verify,
			OK:     ur.Ok,
		}
		if moveErr != nil {
			rec.Err = moveErr.Error()
		}
		_ = j.Append(rec)
	}
	return results, batch, nil
}
