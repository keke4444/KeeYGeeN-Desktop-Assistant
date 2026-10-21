package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m Model) View() string {
	if m.err != nil {
		return m.viewError()
	}
	var b strings.Builder
	b.WriteString(m.viewHeader())
	b.WriteString("\n")
	b.WriteString(m.viewBody())
	if m.showDetailLines() {
		b.WriteString("\n")
		b.WriteString(m.viewDetail())
	}
	b.WriteString("\n")
	b.WriteString(m.viewKeybar())
	return b.String()
}

func (m Model) viewHeader() string {
	w := m.vw()
	title := stTitle.Render(padTo(" Ubuntu 桌面整理 ", w))
	env := stMuted.Render(padTo(" 桌面: "+elideMiddle(m.desktop, w-10), w))

	var cat string
	n := len(m.reg.Categories)
	if n == 0 {
		cat = stMuted.Render(padTo(truncate(" 目标分类: （暂无分类）  先建一个分类文件夹，或写进 config 的 extra_categories", w), w))
	} else {
		idx := m.catIdx
		if idx >= n {
			idx = 0
		}
		if idx < 0 {
			idx = 0
		}
		name := m.reg.Categories[idx].Name
		head := stMuted.Render(" 目标分类: ")
		mid := stAccent.Render(fmt.Sprintf("◀ %s (%d/%d) ▶", truncate(name, maxInt(4, w-34)), idx+1, n))
		cat = padTo(head+mid, w)
		if w >= 88 {
			cat = padTo(head+mid+stMuted.Render("  [Tab] 切换  [1-9] 直选  [c] 单项指定"), w)
		}
	}
	return title + "\n" + env + "\n" + cat
}

func (m Model) viewBody() string {
	switch m.st {
	case stateConfirm:
		return m.viewConfirm()
	case statePick:
		return m.viewPick()
	case stateProgress:
		return m.viewProgress()
	case stateReport:
		return m.viewReport()
	case stateHelp:
		return m.viewHelp()
	default:
		return m.viewList()
	}
}

func (m Model) viewList() string {
	h := m.listHeight()
	var lines []string
	if len(m.files) == 0 {
		lines = append(lines, stMuted.Render(" 桌面很干净，没有需要整理的文件  (r 重新扫描 · q 退出)"))
	} else {
		end := m.offset + h
		if end > len(m.files) {
			end = len(m.files)
		}
		for i := m.offset; i < end; i++ {
			lines = append(lines, m.viewRow(i))
		}
	}
	return padBlock(strings.Join(lines, "\n"), h, m.vw())
}

func (m Model) viewRow(i int) string {
	f := m.files[i]
	w := m.vw()
	sizeW := 10
	nameW := w - 4 - sizeW
	if nameW < 12 {
		nameW = 12
		sizeW = 0
	}

	pointer := " "
	if i == m.cursor {
		pointer = "▸"
	}
	mark := " "
	if m.marked[f.Path] {
		mark = "●"
	} else if f.Ext == "" {
		mark = "◆"
	}

	line := pointer + " " + mark + " " + padTo(truncate(f.Name, nameW), nameW)
	if sizeW > 0 {
		line += " " + fmt.Sprintf("%*s", sizeW, humanSize(f.Size))
	}
	line = padTo(line, w)

	switch {
	case i == m.cursor:
		return stCursor.Render(line)
	case m.marked[f.Path]:
		return stMarked.Render(line)
	default:
		return line
	}
}

func (m Model) viewDetail() string {
	var first string
	if m.status != "" {
		first = stAccent.Render(" " + truncate(m.status, m.vw()-2))
	} else if len(m.files) > 0 && m.cursor < len(m.files) {
		f := m.files[m.cursor]
		first = stMuted.Render(truncate(fmt.Sprintf(" 选中 %s · %s · 修改 %s",
			f.Name, humanSize(f.Size), f.ModTime.Format("2006-01-02 15:04")), m.vw()-2))
	} else {
		first = stMuted.Render(" 选中 —")
	}

	target := "（暂无分类）"
	if c := m.currentCategory(); c != "" {
		target = filepath.Join(m.desktop, m.cfg.RootDir, c)
	}
	second := stMuted.Render(truncate(fmt.Sprintf(" 已标记 %d 项 / 共 %d 项 · 目标目录: %s",
		len(m.marked), len(m.files), target), m.vw()-2))

	return padTo(first, m.vw()) + "\n" + padTo(second, m.vw())
}

func (m Model) viewKeybar() string {
	var text string
	switch m.st {
	case stateConfirm:
		text = " y 确认    n 取消 "
	case stateProgress:
		text = " Ctrl+C 中止 "
	case stateReport:
		text = " d 明细    Enter 返回 "
	case stateHelp:
		text = " ? / Esc 关闭 "
	case statePick:
		text = " ↑↓ 选择    Enter 应用    Esc 取消 "
	default:
		if m.vw() < 80 {
			text = " Space/Enter/Tab/c/r/?/q "
		} else {
			text = " Space 标记  Enter 移动  Tab 切分类  c 改分类  r 重扫  ? 帮助  q 退出 "
		}
	}
	return stKeybar.Render(padTo(text, m.vw()))
}

func (m Model) viewConfirm() string {
	boxW := m.boxWidth()
	total := int64(0)
	cats := map[string]int{}
	for _, it := range m.pending {
		total += it.entry.Size
		cats[it.cat]++
	}

	lines := []string{
		stBoxTitle.Render(fmt.Sprintf("确认移动 %d 个文件 (%s)", len(m.pending), humanSize(total))),
	}
	if len(cats) == 1 {
		lines = append(lines, "目标目录: "+elideMiddle(filepath.Join(m.desktop, m.cfg.RootDir, m.pending[0].cat), boxW-12))
	} else {
		var names []string
		for c, n := range cats {
			names = append(names, fmt.Sprintf("%s ×%d", c, n))
		}
		sort.Strings(names)
		lines = append(lines, "目标分类: "+truncate(strings.Join(names, "  "), boxW-12))
	}
	lines = append(lines, "")

	const maxShow = 5
	for i, it := range m.pending {
		if i >= maxShow {
			lines = append(lines, fmt.Sprintf("… 其余 %d 个", len(m.pending)-maxShow))
			break
		}
		lines = append(lines, "├ "+truncate(it.entry.Name, boxW-5))
	}
	lines = append(lines, "")
	lines = append(lines, "[y] 确认    [n] 取消")

	box := stBox.Width(boxW).Render(strings.Join(lines, "\n"))
	return centerBlock(box, m.listHeight())
}

func (m Model) viewPick() string {
	boxW := m.boxWidth()
	lines := []string{stBoxTitle.Render("选择分类")}
	for i, c := range m.reg.Categories {
		prefix := "  "
		if i == m.pickIdx {
			prefix = "▸ "
		}
		row := prefix + c.Name
		if i == m.pickIdx {
			row = stAccent.Render(row)
		}
		lines = append(lines, truncate(row, boxW-4))
	}
	lines = append(lines, "")
	lines = append(lines, "[Enter] 应用    [Esc] 取消")

	box := stBox.Width(boxW).Render(strings.Join(lines, "\n"))
	return centerBlock(box, m.listHeight())
}

func (m Model) viewProgress() string {
	ok, skip, fail := m.counts()
	total := m.total
	if total == 0 {
		total = 1
	}
	frac := float64(m.done) / float64(total)
	if frac > 1 {
		frac = 1
	}
	barW := m.vw() - 30
	if barW < 10 {
		barW = 10
	}
	if barW > 40 {
		barW = 40
	}
	filled := int(float64(barW) * frac)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)

	spin := spinnerFrames[m.spin%len(spinnerFrames)]
	lines := []string{
		fmt.Sprintf(" %s 正在移动 %d/%d", spin, m.done, m.total),
		fmt.Sprintf(" [%s] %d%%", bar, int(frac*100)),
		" 当前: " + truncate(m.current, m.vw()-8),
		fmt.Sprintf(" 已完成 %d · 跳过 %d · 失败 %d", ok, skip, fail),
	}
	if m.cancelled {
		lines = append(lines, stDanger.Render(" 已请求中止，等待当前文件完成后停止"))
	}
	return padBlock(strings.Join(lines, "\n"), m.listHeight(), m.vw())
}

func (m Model) viewReport() string {
	ok, skip, fail := m.counts()
	head := fmt.Sprintf(" 移动结果：")
	counts := stSuccess.Render(fmt.Sprintf("成功 %d", ok)) + "   " +
		stMuted.Render(fmt.Sprintf("跳过 %d", skip)) + "   " +
		stDanger.Render(fmt.Sprintf("失败 %d", fail))
	lines := []string{stAccent.Render(head), " " + counts, ""}

	if m.showDetails {
		shown := 0
		for _, r := range m.results {
			if r.Err == "" {
				continue
			}
			if shown >= m.listHeight()-5 {
				lines = append(lines, " …")
				break
			}
			reason := r.Err
			line := " - " + filepath.Base(r.Src) + ": " + reason
			if r.Ok {
				lines = append(lines, stMuted.Render(truncate(line, m.vw()-2)))
			} else if r.Skipped {
				lines = append(lines, stMuted.Render(truncate(line, m.vw()-2)))
			} else {
				lines = append(lines, stDanger.Render(truncate(line, m.vw()-2)))
			}
			shown++
		}
		if shown == 0 {
			lines = append(lines, stMuted.Render(" 没有跳过或失败项"))
		}
	} else {
		lines = append(lines, stMuted.Render(" 按 d 查看跳过/失败明细"))
	}
	lines = append(lines, "", stMuted.Render(" Enter 返回列表"))
	return padBlock(strings.Join(lines, "\n"), m.listHeight(), m.vw())
}

func (m Model) viewHelp() string {
	rows := [][2]string{
		{"↑ ↓ / k j", "移动光标"},
		{"g / G", "跳到首项 / 末项"},
		{"Ctrl+u / Ctrl+d", "上翻 / 下翻半页"},
		{"Tab / Shift+Tab", "切换目标分类"},
		{"1 - 9", "直接选择第 N 个分类"},
		{"Space", "标记 / 取消标记"},
		{"a", "全选 / 取消全选"},
		{"c", "为光标项单独指定分类"},
		{"Enter", "移动（有标记则移动全部标记项）"},
		{"r", "重新扫描桌面"},
		{"q / Ctrl+C", "退出"},
	}
	lines := []string{stAccent.Render(" 快捷键"), ""}
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("  %-20s %s", r[0], r[1]))
	}
	lines = append(lines, "", stMuted.Render("  ? / Esc 关闭帮助"))
	return padBlock(strings.Join(lines, "\n"), m.listHeight(), m.vw())
}

func (m Model) viewError() string {
	lines := []string{
		stDanger.Render(" 启动失败"),
		"",
		" " + m.err.Error(),
		"",
		stMuted.Render(" 请检查：" ),
		stMuted.Render("  1. 配置文件中的 desktop 是否指向真实存在的目录"),
		stMuted.Render("  2. 配置的分类名是否符合 Windows 文件夹命名规则"),
		stMuted.Render("  3. 是否有读取桌面目录的权限"),
		"",
		stMuted.Render(" 按 q 退出"),
	}
	return padBlock(strings.Join(lines, "\n"), maxInt(m.height, 12), m.vw())
}

func (m Model) boxWidth() int {
	w := m.vw() - 8
	if w > 72 {
		w = 72
	}
	if w < 28 {
		w = 28
	}
	return w
}

func padBlock(block string, h, w int) string {
	lines := strings.Split(block, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i := range lines {
		lines[i] = padTo(lines[i], w)
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", maxInt(w, 0)))
	}
	return strings.Join(lines, "\n")
}

func centerBlock(block string, h int) string {
	lines := strings.Split(block, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
