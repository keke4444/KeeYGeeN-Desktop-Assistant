package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width < 40 {
			m.width = 40
		}
		if m.height < 10 {
			m.height = 10
		}
		m.ensureVisible()
		return m, nil

	case spinnerMsg:
		if m.st == stateProgress {
			m.spin = (m.spin + 1) % len(spinnerFrames)
			return m, tickSpinner()
		}
		return m, nil

	case moveProgress:
		m.results = append(m.results, msg.res)
		m.done = msg.done
		m.current = msg.name
		return m, waitFor(m.ch)

	case moveFinished:
		m.st = stateReport
		m.ch = nil
		m.current = ""
		m.rescan()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.st {
	case stateConfirm:
		return m.keyConfirm(k)
	case stateProgress:
		return m.keyProgress(k)
	case stateReport:
		return m.keyReport(k)
	case stateHelp:
		return m.keyHelp(k)
	case statePick:
		return m.keyPick(k)
	default:
		return m.keyList(k)
	}
}

func (m Model) keyList(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	page := m.listHeight() / 2
	if page < 1 {
		page = 1
	}
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = len(m.files) - 1
	case "ctrl+u":
		m.cursor -= page
	case "ctrl+d":
		m.cursor += page
	case "tab":
		if n := len(m.reg.Categories); n > 0 {
			m.catIdx = (m.catIdx + 1) % n
		}
	case "shift+tab":
		if n := len(m.reg.Categories); n > 0 {
			m.catIdx = (m.catIdx - 1 + n) % n
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(k.String()[0]-'1')
		if idx < len(m.reg.Categories) {
			m.catIdx = idx
		}
	case " ", "space":
		m.toggleMark()
	case "a":
		m.toggleAll()
	case "c":
		if len(m.reg.Categories) == 0 {
			m.status = "桌面上还没有分类文件夹，先建一个（或写进 config 的 extra_categories）"
			break
		}
		if len(m.files) == 0 {
			break
		}
		m.pickIdx = m.catIdx
		if cur, ok := m.override[m.files[m.cursor].Path]; ok {
			for i, c := range m.reg.Categories {
				if strings.EqualFold(c.Name, cur) {
					m.pickIdx = i
					break
				}
			}
		}
		m.st = statePick
	case "enter":
		if len(m.reg.Categories) == 0 {
			m.status = "桌面上还没有分类文件夹，先建一个（或写进 config 的 extra_categories）"
			break
		}
		tg := m.targets()
		if len(tg) == 0 {
			m.status = "没有可移动的文件"
			break
		}
		m.pending = tg
		m.st = stateConfirm
	case "r":
		m.rescan()
		m.status = "已重新扫描"
	case "/":
		m.status = "过滤功能将在后续版本提供"
	case "?":
		m.st = stateHelp
	}

	m.clampCursor()
	return m, nil
}

func (m Model) keyConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "y", "enter":
		return m, m.beginMove()
	case "n", "esc", "q":
		m.st = stateList
		m.pending = nil
	}
	return m, nil
}

func (m Model) keyProgress(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" && m.cancel != nil {
		m.cancel.Store(true)
		m.cancelled = true
	}
	return m, nil
}

func (m Model) keyReport(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "d":
		m.showDetails = !m.showDetails
	case "enter", "esc", "q":
		m.st = stateList
		m.showDetails = false
		if ok, skip, fail := m.counts(); fail > 0 {
			m.status = fmt.Sprintf("上次移动：成功 %d · 跳过 %d · 失败 %d", ok, skip, fail)
		} else {
			m.status = fmt.Sprintf("上次移动：成功 %d · 跳过 %d", ok, skip)
		}
	}
	return m, nil
}

func (m Model) keyHelp(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "?", "esc", "q", "enter":
		m.st = stateList
	}
	return m, nil
}

func (m Model) keyPick(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if n := len(m.reg.Categories); n > 0 {
			m.pickIdx = (m.pickIdx - 1 + n) % n
		}
	case "down", "j":
		if n := len(m.reg.Categories); n > 0 {
			m.pickIdx = (m.pickIdx + 1) % n
		}
	case "enter":
		if len(m.files) > 0 && m.pickIdx < len(m.reg.Categories) {
			m.override[m.files[m.cursor].Path] = m.reg.Categories[m.pickIdx].Name
		}
		m.st = stateList
	case "esc", "q":
		m.st = stateList
	}
	return m, nil
}

func (m *Model) clampCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.files) {
		m.cursor = len(m.files) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
}

func (m Model) counts() (ok, skipped, failed int) {
	for _, r := range m.results {
		switch {
		case r.Skipped:
			skipped++
		case r.Ok && r.Err == "":
			ok++
		case r.Ok:
			ok++ // 成功但源文件未删除，仍算成功，明细里会提示
		default:
			failed++
		}
	}
	return
}
