package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"dtidy/internal/category"
	"dtidy/internal/config"
	"dtidy/internal/fsops"
	"dtidy/internal/journal"
	"dtidy/internal/platform"
	"dtidy/internal/tui"
)

const version = "0.3.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "undo":
			cmdUndo(os.Args[2:])
		case "config":
			cmdConfig(os.Args[2:])
		case "scan":
			cmdScan()
		case "version", "-v", "--version":
			fmt.Printf("dtidy %s\n", version)
		case "help", "-h", "--help":
			usage()
		default:
			fmt.Fprintf(os.Stderr, "未知命令：%s\n\n", os.Args[1])
			usage()
			os.Exit(2)
		}
		return
	}
	runTUI()
}

func usage() {
	fmt.Print(`dtidy — 桌面归类工具（桌面 = /home，分类文件夹直接挂在桌面下）

用法:
  dtidy                     启动 TUI（默认）
  dtidy scan                打印桌面文件与分类（调试用）
  dtidy undo [--batch ID]   撤销最近/指定批次
  dtidy undo --list         列出最近批次
  dtidy config path         输出配置文件路径
  dtidy config init [--force] 生成默认配置文件
  dtidy version             版本信息

说明:
  · 只移动文件，从不删除用户文件（复制 → 校验 → 重命名 → 删源）
  · 分类 = 桌面上已有的一级文件夹 + 配置补充；非法分类名直接报错
  · 每次移动写入 JSONL 日志，可撤销
`)
}

func runTUI() {
	cfg, cfgPath, err := config.Load()
	if err != nil {
		fatal(err)
	}
	desktop, err := resolveDesktop(cfg)
	if err != nil {
		fatal(err)
	}

	reg, err := category.Build(filepath.Join(desktop, cfg.RootDir), cfg.DiscoverCategories,
		cfg.ExtraCategories, cfg.ExcludeCategories)
	if err != nil {
		fatal(err)
	}

	j, err := journal.Open()
	if err != nil {
		fatal(err)
	}

	model := tui.New(desktop, cfg, reg, j, newBatchID())
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fatal(err)
	}

	fmt.Printf("\n配置: %s\n日志: %s\n", cfgPath, j.Path())
}

func resolveDesktop(cfg *config.Config) (string, error) {
	if v := strings.TrimSpace(cfg.Desktop); v != "" {
		p := os.ExpandEnv(v)
		fi, err := os.Stat(p)
		if err != nil || !fi.IsDir() {
			return "", fmt.Errorf("配置的 desktop 不是有效目录: %s", p)
		}
		return p, nil
	}
	p, err := platform.DesktopDir()
	if err != nil {
		return "", fmt.Errorf("%w\n提示：可在配置文件中手动指定 desktop = \"C:\\\\Users\\\\你\\\\Desktop\"", err)
	}
	if fi, statErr := os.Stat(p); statErr != nil || !fi.IsDir() {
		return "", fmt.Errorf("解析到的桌面目录不存在: %s", p)
	}
	return p, nil
}

func cmdScan() {
	cfg, _, err := config.Load()
	if err != nil {
		fatal(err)
	}
	desktop, err := resolveDesktop(cfg)
	if err != nil {
		fatal(err)
	}
	reg, err := category.Build(filepath.Join(desktop, cfg.RootDir), cfg.DiscoverCategories,
		cfg.ExtraCategories, cfg.ExcludeCategories)
	if err != nil {
		fatal(err)
	}
	res, err := fsops.Scan(desktop, cfg.ShowHidden, cfg.Ignore)
	if err != nil {
		fatal(err)
	}

	fmt.Printf("桌面: %s\n", desktop)
	fmt.Printf("分类根: %s\n", orNone(cfg.RootDir))
	fmt.Printf("分类 (%d): %s\n", len(reg.Categories), strings.Join(reg.Names(), " / "))
	fmt.Printf("文件 (%d):\n", len(res.Files))
	for _, f := range res.Files {
		fmt.Printf("  %-40s %10s\n", f.Name, humanSize(f.Size))
	}
	if len(res.Dirs) > 0 {
		fmt.Printf("目录 (%d): %s\n", len(res.Dirs), strings.Join(res.Dirs, " / "))
	}
}

func cmdUndo(args []string) {
	batch := ""
	list := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--batch", "-b":
			if i+1 >= len(args) {
				fatal(fmt.Errorf("--batch 需要批次 ID"))
			}
			batch = args[i+1]
			i++
		case "--list", "-l":
			list = true
		default:
			fatal(fmt.Errorf("未知参数：%s", args[i]))
		}
	}

	cfg, _, err := config.Load()
	if err != nil {
		fatal(err)
	}
	j, err := journal.Open()
	if err != nil {
		fatal(err)
	}

	if list {
		records, err := j.All()
		if err != nil {
			fatal(err)
		}
		seen := map[string]bool{}
		var batches []string
		for i := len(records) - 1; i >= 0; i-- {
			r := records[i]
			if r.Action != "move" || !r.OK || seen[r.Batch] {
				continue
			}
			seen[r.Batch] = true
			batches = append(batches, r.Batch)
		}
		if len(batches) == 0 {
			fmt.Println("没有可撤销的批次")
			return
		}
		fmt.Println("最近批次:")
		for i, b := range batches {
			if i >= 10 {
				break
			}
			fmt.Printf("  %s\n", b)
		}
		return
	}

	results, usedBatch, err := journal.Undo(j, batch, cfg.Verify)
	if err != nil {
		fatal(err)
	}

	ok, skipped, failed := 0, 0, 0
	for _, r := range results {
		switch {
		case r.Ok:
			ok++
		case r.Skipped:
			skipped++
		default:
			failed++
		}
	}
	fmt.Printf("撤销批次 %s：成功 %d · 跳过 %d · 失败 %d\n", usedBatch, ok, skipped, failed)
	for _, r := range results {
		if r.Ok {
			continue
		}
		fmt.Printf("  - %s: %s\n", filepath.Base(r.Dst), r.Reason)
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func cmdConfig(args []string) {
	if len(args) == 0 {
		fatal(fmt.Errorf("用法: dtidy config path | init [--force]"))
	}
	switch args[0] {
	case "path":
		p, err := config.Path()
		if err != nil {
			fatal(err)
		}
		fmt.Println(p)
	case "init":
		force := false
		for _, a := range args[1:] {
			if a == "--force" || a == "-f" {
				force = true
			}
		}
		p, err := config.Path()
		if err != nil {
			fatal(err)
		}
		if _, statErr := os.Stat(p); statErr == nil && !force {
			fmt.Printf("配置文件已存在：%s\n（如需覆盖请加 --force）\n", p)
			return
		}
		cfg := config.Default()
		if err := cfg.Save(p); err != nil {
			fatal(err)
		}
		fmt.Printf("已生成默认配置：%s\n", p)
	default:
		fatal(fmt.Errorf("未知的 config 子命令：%s", args[0]))
	}
}

func newBatchID() string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

func humanSize(n int64) string {
	const unit = int64(1024)
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := unit, 0
	for n/div >= unit && exp < 4 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTP"[exp])
}

func orNone(s string) string {
	if s == "" {
		return "（无，分类直接挂在桌面下）"
	}
	return s
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "错误: %v\n", err)
	os.Exit(1)
}
