package tui

import "github.com/charmbracelet/lipgloss"

// Ubuntu 品牌色 + 256 色降级由 lipgloss 自动处理（终端不支持真彩时近似）。
var (
	colPrimary     = lipgloss.Color("#772953")
	colPrimaryDark = lipgloss.Color("#5E2750")
	colAccent      = lipgloss.Color("#E95420")
	colAccentLight = lipgloss.Color("#F0873A")
	colText        = lipgloss.Color("#F2F2F2")
	colMuted       = lipgloss.Color("#AEA79F")
	colDanger      = lipgloss.Color("#C7162B")
	colSuccess     = lipgloss.Color("#0E8420")

	stTitle    = lipgloss.NewStyle().Background(colPrimary).Foreground(colAccentLight).Bold(true)
	stCursor   = lipgloss.NewStyle().Background(colPrimary).Foreground(colText)
	stMarked   = lipgloss.NewStyle().Foreground(colAccent)
	stAccent   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stMuted    = lipgloss.NewStyle().Foreground(colMuted)
	stDanger   = lipgloss.NewStyle().Foreground(colDanger).Bold(true)
	stSuccess  = lipgloss.NewStyle().Foreground(colSuccess).Bold(true)
	stKeybar   = lipgloss.NewStyle().Background(colAccent).Foreground(lipgloss.Color("#FFFFFF"))
	stBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
	stBoxTitle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
)
