// Package tui 实现 bubbletea 主界面：顶部搜索框、左侧结果列表、
// 右侧预览面板（代码高亮 / 图片），底部状态栏。
package tui

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
	"scx-rg/internal/search"
)

// Mode 搜索模式。
type Mode int

const (
	ModeFiles Mode = iota
	ModeContent
)

func (m Mode) String() string {
	if m == ModeContent {
		return "内容"
	}
	return "文件"
}

// Config 启动配置。
type Config struct {
	Root        string
	Mode        Mode
	Debounce    time.Duration
	ImgProto    preview.Protocol
	RgAvailable bool
}

type (
	// debounceMsg 防抖计时到期；version 过期则丢弃。
	debounceMsg struct{ version uint64 }
	// resultsMsg 异步搜索结果。
	resultsMsg struct {
		version uint64
		results []search.Result
		err     error
	}
	// previewMsg 异步预览渲染结果。
	previewMsg struct {
		path     string
		rendered preview.Rendered
		err      error
	}
)

var errNoRg = errors.New("未找到 rg，请先安装 ripgrep（brew install ripgrep）")

// Model bubbletea 主模型。
type Model struct {
	cfg  Config
	root string

	width, height int
	listW, prevW  int

	mode    Mode
	input   textinput.Model
	version uint64 // 查询版本号，防抖与过期结果的判废依据

	results   []search.Result
	sel       int
	offset    int
	searching bool
	searchErr error

	vp          viewport.Model
	prevPath    string
	prevJump    int
	prevLines   int
	prevKind    string
	prevLang    string
	prevLoading bool

	picked string
}

// New 创建主模型。
func New(cfg Config) *Model {
	if cfg.Debounce <= 0 {
		cfg.Debounce = 200 * time.Millisecond
	}
	ti := textinput.New()
	ti.Prompt = "❯ "
	ti.Placeholder = "输入关键词，实时搜索…"
	ti.CharLimit = 256
	ti.PromptStyle = stylePrompt
	ti.Focus()

	m := &Model{cfg: cfg, root: cfg.Root, mode: cfg.Mode, input: ti}
	if m.mode == ModeContent && !cfg.RgAvailable {
		m.mode = ModeFiles
	}
	return m
}

// Init 启动时立即触发一次空查询搜索（files 模式列出全部文件）。
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tickDebounce(m.version, 0))
}

func tickDebounce(v uint64, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return debounceMsg{version: v} })
}

func (m *Model) provider() search.Provider {
	if m.mode == ModeContent {
		if m.cfg.RgAvailable {
			return search.RipgrepProvider{}
		}
		return nil
	}
	return search.FilesProvider{}
}

// runSearch 基于当前查询发起异步搜索。
func (m *Model) runSearch() tea.Cmd {
	v := m.version
	p := m.provider()
	if p == nil {
		m.searching = true
		return func() tea.Msg { return resultsMsg{version: v, err: errNoRg} }
	}
	m.searching = true
	m.searchErr = nil
	q := m.input.Value()
	root := m.root
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := p.Search(ctx, root, q)
		return resultsMsg{version: v, results: res, err: err}
	}
}

func (m *Model) panelH() int      { return max(0, m.height-4) }
func (m *Model) listVisible() int { return max(0, m.panelH()-3) }

func (m *Model) adjustOffset() {
	vis := m.listVisible()
	if m.sel < m.offset {
		m.offset = m.sel
	}
	if m.sel >= m.offset+vis {
		m.offset = m.sel - vis + 1
	}
	m.offset = max(0, m.offset)
}

// followSelection 让预览跟随当前选中项；同文件不同行只做跳转，不重新渲染。
func (m *Model) followSelection() tea.Cmd {
	if len(m.results) == 0 || m.sel >= len(m.results) {
		m.vp.SetContent("")
		m.prevPath = ""
		return nil
	}
	r := m.results[m.sel]
	if r.Path == m.prevPath {
		if r.Line > 0 && r.Line != m.prevJump {
			m.prevJump = r.Line
			m.scrollToJump(r.Line, m.prevLines)
		}
		return nil
	}
	m.prevPath = r.Path
	m.prevJump = r.Line
	m.prevLoading = true
	cols, rows := m.previewSize()
	proto := m.cfg.ImgProto
	jump := r.Line
	abs := filepath.Join(m.root, r.Path)
	return func() tea.Msg {
		ren, err := preview.Render(abs, cols, rows, proto, jump)
		return previewMsg{path: r.Path, rendered: ren, err: err}
	}
}

// followSelectionReload 强制重新渲染当前选中项（窗口尺寸变化后调用）。
func (m *Model) followSelectionReload() tea.Cmd {
	m.prevPath = ""
	return m.followSelection()
}

func (m *Model) previewSize() (cols, rows int) {
	cols = max(0, m.prevW-2)
	rows = max(0, m.panelH()-3)
	return
}

func (m *Model) scrollToJump(jump, totalLines int) {
	if jump <= 0 || totalLines <= 0 || m.vp.Height <= 0 {
		return
	}
	y := jump - 1 - max(1, m.vp.Height/3)
	y = max(0, y)
	y = min(y, max(0, totalLines-m.vp.Height))
	m.vp.SetYOffset(y)
}

// PickedPath 返回 Enter 选中的绝对路径；未选中返回空。
func (m *Model) PickedPath() string { return m.picked }

// RenderOnce 不启动事件循环，同步跑完搜索与预览后渲染一帧（--once 调试用）。
func (m *Model) RenderOnce(w, h int, query, focus string) string {
	_, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	if query != "" {
		m.input.SetValue(query)
	}
	m.drain(m.runSearch())
	if focus != "" {
		m.results = []search.Result{{Path: focus}}
		m.sel = 0
		m.drain(m.followSelection())
	}
	return m.View()
}

func (m *Model) drain(cmd tea.Cmd) {
	for i := 0; cmd != nil && i < 16; i++ {
		msg := cmd()
		if msg == nil {
			return
		}
		_, next := m.Update(msg)
		cmd = next
	}
}
