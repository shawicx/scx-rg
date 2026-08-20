// Package tui 实现 bubbletea 主界面：顶部搜索框、左侧结果列表、
// 右侧预览面板（代码高亮 / 图片），底部状态栏。
package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/logs"
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
	// Title 非空时替代头部默认标题（如 "docker:web"）。
	Title string
	// PickLine 为 true 时 Enter 输出选中行文本而非文件路径（临时日志快照等场景）。
	PickLine bool
	// FollowFile 非空时进入跟随模式：轮询该文件增长并自动刷新当前查询（tail -f 式）。
	FollowFile string

	// 源选择器（scx-rg docker / scx-rg k8s 无参数进入）
	PickerKind  string // "docker" | "kubectl"
	SnapshotDir string // 快照落盘目录（由 main 创建与清理）
	FollowPick  bool   // 选中目标后跟随而非一次性快照
	LogTail     int    // 抓取行数上限（0 = 默认 100000）
	// 以下可注入 fake 以便测试；为 nil 时调用 logs 包真实实现。
	ListSources func(ctx context.Context, kind string) ([]logs.Source, error)
	FetchLog    func(ctx context.Context, t logs.Target) (string, error)
	FollowLog   func(ctx context.Context, t logs.Target, path string) error
}

type (
	// debounceMsg 防抖计时到期；version 过期则丢弃。
	debounceMsg struct{ version uint64 }
	// resultsMsg 同步搜索的完整结果。
	resultsMsg struct {
		version uint64
		results []search.Result
		err     error
	}
	// resultMsg 流式搜索的单条结果。
	resultMsg struct {
		version uint64
		result  search.Result
	}
	// streamDoneMsg 流式搜索结束（完成 / 取消 / 封顶）。
	streamDoneMsg struct{ version uint64 }
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
	// fallbackActive：文件模式无命中后自动切换到全文搜索结果展示
	fallbackActive bool

	// 流式搜索状态：cancelSearch 取消当前流（杀掉 rg 进程），
	// streamCh 供 waitForResult 消息链继续消费。
	cancelSearch context.CancelFunc
	streamCh     <-chan search.Result

	vp          viewport.Model
	prevPath    string
	prevJump    int
	prevLines   int
	prevKind    string
	prevLang    string
	prevLoading bool

	// 跟随模式状态
	followSize int64  // 上次观测到的快照文件大小
	followKeep string // 刷新时待恢复的选中项（path:line）
	onceMode   bool   // RenderOnce 调试路径：禁用周期性 tick

	// 可视化筛选栏（Ctrl+T）：客户端过滤，不重新抓取
	rangeBar    bool             // 筛选栏打开且聚焦
	rangeSeg    int              // 0=时间 1=条数
	rangeSel    [2]int           // 两段的光标位置
	filterDur   time.Duration    // 时间筛选，0=全部
	filterCap   int              // 条数封顶（保留最新 N 条），0=全部
	raw         []search.Result  // 未过滤结果缓冲（与流式封顶一致）
	tsOK        bool             // 本轮结果中检测到行首时间戳
	liveTicking bool             // 实时滑动窗口的 tick 链运转中
	now         func() time.Time // 可注入时钟（测试模拟时间流逝）

	// 源选择器状态
	picking      bool          // 处于「选目标」阶段
	pickerKind   string        // docker | kubectl
	pickerSrcs   []logs.Source // 全量列表
	pickerView   []logs.Source // 过滤后的可见列表
	pickerName   string        // Enter 选中的目标名
	pickLoading  bool          // 抓取中
	listLoading  bool          // 列表加载中
	snapshotDir  string
	followPick   bool
	cancelFollow context.CancelFunc // 跟随进程的取消句柄

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
	if m.mode == ModeContent && !cfg.RgAvailable && cfg.PickerKind == "" {
		m.mode = ModeFiles
	}
	if cfg.FollowFile != "" {
		if st, err := os.Stat(cfg.FollowFile); err == nil {
			m.followSize = st.Size() // 以当前大小为基线，之后增长才触发刷新
		}
	}
	if cfg.PickerKind != "" {
		m.picking = true
		m.pickerKind = cfg.PickerKind
		m.snapshotDir = cfg.SnapshotDir
		m.followPick = cfg.FollowPick
		m.listLoading = true
		m.mode = ModeContent
		ti.Placeholder = "输入名称过滤，实时匹配…"
	}
	return m
}

// Init 启动时立即触发一次空查询搜索（files 模式列出全部文件）；
// 选择器模式改为加载源列表，不发搜索。
func (m *Model) Init() tea.Cmd {
	if m.picking {
		return tea.Batch(textinput.Blink, m.loadPicker())
	}
	cmds := []tea.Cmd{textinput.Blink, tickDebounce(m.version, 0)}
	if m.following() {
		cmds = append(cmds, followTick())
	}
	return tea.Batch(cmds...)
}

func tickDebounce(v uint64, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return debounceMsg{version: v} })
}

// nowFunc 当前时间；测试可注入假时钟。
func (m *Model) nowFunc() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func (m *Model) provider() search.Provider {
	if m.mode == ModeContent {
		if m.cfg.RgAvailable {
			return search.RipgrepProvider{}
		}
		return nil
	}
	return search.FilesProvider{UseRg: m.cfg.RgAvailable}
}

// runSearch 基于当前查询发起搜索：先取消上一轮（流式会立刻杀掉 rg 进程），
// 清空列表与预览，再按 Provider 类型走同步或流式路径。
func (m *Model) runSearch() tea.Cmd {
	if m.cancelSearch != nil {
		m.cancelSearch()
		m.cancelSearch = nil
	}
	m.streamCh = nil
	m.version++
	v := m.version

	m.results = nil
	m.raw = nil
	m.tsOK = false
	m.sel, m.offset = 0, 0
	m.vp.SetContent("")
	m.prevPath = ""
	m.searchErr = nil
	m.fallbackActive = false

	p := m.provider()
	if p == nil {
		m.searching = true
		return func() tea.Msg { return resultsMsg{version: v, err: errNoRg} }
	}
	m.searching = true

	if _, ok := p.(search.StreamProvider); ok {
		return m.startStreamSearch()
	}

	sync := p.(search.SyncProvider)
	q := m.input.Value()
	root := m.root
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := sync.Search(ctx, root, q)
		return resultsMsg{version: v, results: res, err: err}
	}
}

// startStreamSearch 发起内容流式搜索（runSearch 与文件名回退共用）。
// 沿用当前 version，不重置列表；调用前须已取消旧搜索。
func (m *Model) startStreamSearch() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := (search.RipgrepProvider{}).SearchStream(ctx, m.root, m.input.Value())
	if err != nil {
		cancel()
		m.searching = false
		m.searchErr = err
		return nil
	}
	m.cancelSearch = cancel
	m.streamCh = ch
	m.searching = true
	return m.waitForResult(ch, m.version)
}

// startFallbackStream 文件名搜索零命中时的全文回退。
func (m *Model) startFallbackStream() tea.Cmd {
	m.fallbackActive = true
	return m.startStreamSearch()
}

// waitForResult 阻塞取一条流式结果；channel 关闭时发出结束消息。
func (m *Model) waitForResult(ch <-chan search.Result, v uint64) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return streamDoneMsg{version: v}
		}
		return resultMsg{version: v, result: r}
	}
}

// stopSearch 结束当前流：取消进程、清空句柄。幂等。
func (m *Model) stopSearch() {
	if m.cancelSearch != nil {
		m.cancelSearch()
		m.cancelSearch = nil
	}
	m.streamCh = nil
	m.searching = false
}

func (m *Model) panelH() int {
	h := m.height - 4
	if m.rangeBar {
		h -= 2 // 筛选栏占两行
	}
	return max(0, h)
}
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
	// 同文件同行才免重渲染；行变化（内容模式）需按新行号重开窗口——
	// 窗口化渲染里真实行号≠物理行号，仅滚动会错位
	if r.Path == m.prevPath && r.Line == m.prevJump {
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
	m.onceMode = true
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

// drain 同步驱动 cmd/msg 链直到结束，模拟 bubbletea 事件循环（展开 BatchMsg）。
// 光标闪烁等纯装饰性周期消息会被丢弃，否则其自续链会让同步驱动器永远跑不完。
// 上限足够消费完一条流的最大结果数。
func (m *Model) drain(cmd tea.Cmd) {
	pending := []tea.Cmd{cmd}
	for i := 0; len(pending) > 0 && i < 1<<20; i++ {
		c := pending[0]
		pending = pending[1:]
		if c == nil {
			continue
		}
		msg := c()
		if msg == nil {
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			pending = append(pending, batch...)
			continue
		}
		if _, ok := msg.(cursor.BlinkMsg); ok {
			continue
		}
		_, next := m.Update(msg)
		pending = append(pending, next)
	}
}
