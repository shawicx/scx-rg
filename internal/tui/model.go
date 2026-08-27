// Package tui 实现 bubbletea 主界面：顶部搜索框、左侧结果列表、
// 右侧预览面板（代码高亮 / 图片），底部状态栏。
package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

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

	// Candidates 非空进入 finder 模式（--provider stdin / docker-ps）：
	// 候选行本地模糊过滤，Enter 输出原行文本（应同时设 PickLine）。
	Candidates []search.Candidate
	// FinderName finder 模式来源标签（如 "stdin" / "docker"），状态栏显示。
	FinderName string
	// IgnoreDirs 额外忽略的目录名（来自 config.toml 的 ignore），枚举两条路径都生效。
	IgnoreDirs []string

	// 编辑器集成（config.toml [editor]）：Command 为空时 Ctrl+E 键位隐藏。
	EditorCommand string
	EditorArgs    []string

	// GitFiles 拉取变更文件集（Git 筛选）；nil 时调用 search 包真实 git。
	GitFiles func(ctx context.Context, root string, staged bool) ([]string, error)

	// HistorySize 历史保留条数（0 = 默认 100）；ShowBlame 状态栏 blame 摘要
	// 默认开关（Ctrl+B 可即时切换）。
	HistorySize int
	ShowBlame   bool

	// BlameFetch 拉取整份 blame porcelain 输出；nil 时调用 search 包真实 git。
	BlameFetch func(ctx context.Context, root, rel string) (string, error)

	// PipeRun 执行管道命令（测试注入）；nil 时真实 sh -c。
	PipeRun func(cmdStr, dir, stdin string) (string, error)

	// GitShow 取 commit 详情（测试注入）；nil 时调用 search 包真实 git。
	GitShow func(ctx context.Context, root, hash string) (string, error)

	// NvimSend 发送按键到 nvim --server（测试注入）；nil 时真实 exec。
	NvimSend func(server, keys string) error

	// ast-grep 替换注入（测试）；nil 时调用 search 包真实实现。
	GitClean func(ctx context.Context, root string) (bool, error)
	AstScan  func(ctx context.Context, root, pattern, rewrite string) ([]search.AstMatch, error)
	AstApply func(root string, matches []search.AstMatch) (int, error)

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
	// matchExact 文件模式的精确/模糊匹配切换（Ctrl+F）：精确=分词须为完整子串
	matchExact bool
	// matchLiteral 内容/全文回退模式的字面量/正则切换（Ctrl+F）：字面量=rg -F
	matchLiteral bool

	results   []search.Result
	sel       int
	offset    int
	searching bool
	searchErr error
	// fallbackActive：文件模式无命中后自动切换到全文搜索结果展示
	fallbackActive bool
	// finder：静态候选模式（--provider stdin / docker-ps），Tab 切模式禁用，
	// Enter 输出原行文本
	finder bool
	// marked：Ctrl+Space 标记的多选项（key = resultKey，path:line），
	// Enter 时输出全部标记项；Esc 在输入清空后先清标记
	marked map[string]bool
	// helpOverlay：帮助浮层打开（? 空输入时 / F1），任意键关闭
	helpOverlay bool
	// palette：命令面板（: 空输入时打开）——过滤词独立于主输入
	paletteOpen  bool
	paletteQuery string
	paletteSel   int
	// themePreset 会话内当前命名主题（命令面板循环切换的游标与展示）
	themePreset string
	// 搜索历史：存储序旧→新（尾部最新）；浮层展示倒序
	history     []string
	historyOpen bool
	historySel  int

	// blame 状态栏摘要（M7-2）：blameOn 开关（Ctrl+B / config [git]），
	// blameText 当前选中行的摘要，缓存按 文件+mtime（见 blame.go）
	blameOn     bool
	blameText   string
	blameCache  *blameCache
	blameActive string // 正在拉取的 path:line（回包判废）

	// gitLog 历史搜索模式（M7-4，命令面板「Git 历史」进入）
	gitLog bool

	// 管道输出（M7-3）：| 空输入打开命令输入浮层，输入独立于主搜索框
	pipeOpen  bool
	pipeInput string

	// 多目录 workspace（M8-2）：主目录仍是 cfg.Root（相对路径结果），
	// 额外目录的结果用绝对路径；命令面板「添加搜索目录」维护
	extraRoots []string
	dirOpen    bool
	dirInput   string

	// ast-grep 替换（M8-3）：两段输入浮层 → 匹配列表模态（y/n/a）
	replaceOpen    bool
	replaceStage   int
	replacePattern string
	replaceRewrite string
	astMode        bool
	astMatches     []search.AstMatch

	// 流式搜索状态：cancelSearch 取消当前流（杀掉 rg 进程），
	// streamCh 供 waitForResult 消息链继续消费。
	cancelSearch context.CancelFunc
	streamCh     <-chan search.Result

	vp       viewport.Model
	prevPath string
	prevJump int
	// prevCustom 预览面板当前是自定义内容（管道输出等）：prevPath 为空
	// 但不能回落到「选中后预览」占位提示
	prevCustom  bool
	prevLines   int
	prevKind    string
	prevLang    string
	prevLoading bool
	// imgActive 当前预览内容含 kitty 图形序列：overlay 图形不随文本替换消失，
	// 切到其他内容时须显式注入删除序列（见 setPreviewContent / previewView）
	imgActive bool
	// 预览渲染缓存（切选回访免重渲）与可注入渲染函数（默认 preview.Render，测试可换 fake 计数）
	prevCache  *preview.Cache
	renderFile func(path string, cols, rows int, proto preview.Protocol, jump int, query string) (preview.Rendered, error)

	// 跟随模式状态
	followSize int64  // 上次观测到的快照文件大小
	followKeep string // 刷新时待恢复的选中项（path:line）
	onceMode   bool   // RenderOnce 调试路径：禁用周期性 tick

	// 可视化筛选栏（Ctrl+T）：客户端过滤，不重新抓取
	rangeBar    bool             // 筛选栏打开且聚焦
	rangeSeg    int              // 0=时间 1=条数 2=Git
	rangeSel    [3]int           // 各段的光标位置
	filterDur   time.Duration    // 时间筛选，0=全部
	filterCap   int              // 条数封顶（保留最新 N 条），0=全部
	raw         []search.Result  // 未过滤结果缓冲（与流式封顶一致）
	tsOK        bool             // 本轮结果中检测到行首时间戳
	windowed    bool             // 日志模式：命中超窗，列表冻结待流结束重算为最新窗口
	liveTicking bool             // 实时滑动窗口的 tick 链运转中
	now         func() time.Time // 可注入时钟（测试模拟时间流逝）

	// Git 筛选（Ctrl+T 第三段）：gitKnown 标记探测完成（含失败），
	// gitOK 决定第三段是否可见
	gitFilter  int             // 0=全部 1=仅改动 2=仅暂存
	gitAllow   map[string]bool // 生效文件集，nil=不过滤
	gitKnown   bool
	gitOK      bool
	gitLoading bool

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

	// 复制/翻页
	writeClipboard func(s string) error // 默认 OSC 52 → /dev/tty，可注入 fake
	notice         string               // 状态栏临时提示（复制成功等）

	picked string
}

// New 创建主模型。
func New(cfg Config) *Model {
	if cfg.Debounce <= 0 {
		cfg.Debounce = 200 * time.Millisecond
	}
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "输入关键词，实时搜索..."
	ti.CharLimit = 256
	tiStyles := ti.Styles()
	tiStyles.Focused.Prompt = stylePrompt
	tiStyles.Blurred.Prompt = stylePrompt
	ti.SetStyles(tiStyles)
	ti.Focus()

	m := &Model{cfg: cfg, root: cfg.Root, mode: cfg.Mode, input: ti, themePreset: "default",
		history: loadHistory(), blameOn: cfg.ShowBlame, blameCache: newBlameCache()}
	m.marked = map[string]bool{}
	m.prevCache = preview.NewCache(32)
	m.renderFile = preview.Render
	if len(cfg.Candidates) > 0 {
		m.finder = true
		m.mode = ModeFiles // 列表行为与文件模式一致；Tab 已禁用
	}
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
		ti.Placeholder = "输入名称过滤，实时匹配..."
	} else if cfg.PickLine {
		m.updatePlaceholder() // 直达日志路径（docker <名字> / --follow）：初始即日志语义
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

// absPath 结果路径 → 绝对路径：额外目录的结果本就是绝对路径，
// 主目录结果相对 cfg.Root。
func (m *Model) absPath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(m.root, p)
}

// roots 全部搜索根：主目录在前；无额外目录返回 nil（单目录语义不变）。
func (m *Model) roots() []string {
	if len(m.extraRoots) == 0 {
		return nil
	}
	return append([]string{m.root}, m.extraRoots...)
}

func (m *Model) provider() search.Provider {
	if m.gitLog {
		return search.GitLogProvider{}
	}
	if m.finder {
		return search.ListProvider{Candidates: m.cfg.Candidates, Exact: m.matchExact}
	}
	if m.mode == ModeContent {
		if m.cfg.RgAvailable {
			// 日志场景（PickLine：docker/k8s 快照、--follow）空查询放行，
			// rg 空模式匹配每一行——「不输入即看全部」；普通内容模式维持短路。
			return search.RipgrepProvider{Roots: m.roots(), AllowEmptyQuery: m.cfg.PickLine}
		}
		return nil
	}
	return search.FilesProvider{UseRg: m.cfg.RgAvailable, Exact: m.matchExact,
		IgnoreExtra: m.cfg.IgnoreDirs, Allow: m.gitAllow, Roots: m.roots()}
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
	m.windowed = false
	m.notice = ""
	m.sel, m.offset = 0, 0
	m.vp.SetContent("")
	m.prevPath = ""
	m.prevCustom = false
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

// startStreamSearch 发起流式搜索（runSearch 与文件名回退共用；provider
// 由 provider() 决定——rg 内容搜索或 git log -G 历史）。
// 沿用当前 version，不重置列表；调用前须已取消旧搜索。
// rg 查询不是合法正则时自动按字面量兜底——用户搜 log.error( 这类含元字符的
// 文本不该先撞一个 regex parse error 再手动切换。
func (m *Model) startStreamSearch() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	var sp search.StreamProvider
	switch p := m.provider().(type) {
	case search.RipgrepProvider:
		literal := m.matchLiteral
		if !literal {
			if _, err := regexp.Compile(m.input.Value()); err != nil {
				literal = true
				m.notice = "非合法正则，已按字面量搜索"
			}
		}
		p.Literal = literal
		sp = p
	case search.StreamProvider:
		sp = p
	default:
		// 文件名零命中回退：provider 仍是同步的 FilesProvider，
		// 流式对象固定为 rg 内容搜索
		sp = search.RipgrepProvider{Literal: m.matchLiteral}
	}
	ch, err := sp.SearchStream(ctx, m.root, m.input.Value())
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
		h -= m.rangeBarH() // 时间+条数两行，git 仓库内追加 Git 段
	}
	return max(0, h)
}

// frameW 帧的最大宽度：比终端少 1 列。帧行若恰好占满终端宽度，行尾会压在
// 最后一列（wrap-pending 边界），部分终端引擎（如 Termius 的本地终端）在
// 高频局部重绘下对该边界的处理有缺陷，会造成错位鬼影；收窄 1 列后行尾不再
// 触碰最后一列，且每行会附带「清除行尾」序列，重绘变为自清洁。
func (m *Model) frameW() int      { return max(0, m.width-1) }
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
// 先查渲染缓存，命中则同步应用；未命中异步渲染并在成功后写回缓存。
func (m *Model) followSelection() tea.Cmd {
	if len(m.results) == 0 || m.sel >= len(m.results) {
		m.setPreviewContent("")
		m.prevPath = ""
		m.prevCustom = false
		m.prevCustom = false
		return nil
	}
	r := m.results[m.sel]
	// 同文件同行才免重渲染；行变化（内容模式）需按新行号重开窗口——
	// 窗口化渲染里真实行号≠物理行号，仅滚动会错位
	if r.Path == m.prevPath && r.Line == m.prevJump {
		return m.requestBlame() // 免重渲染也要刷新 blame 摘要
	}
	m.prevPath = r.Path
	m.prevJump = r.Line
	m.prevLoading = true
	if m.gitLog {
		return m.loadCommitDetail(r.Path, r.Detail)
	}
	if m.finder {
		if abs := m.finderPath(r.Path); abs != "" {
			// 候选恰是存在的文件路径（fd | scx-rg --provider stdin 场景）：正常预览
			return m.renderSelectionPreview(r, abs)
		}
		// 普通候选行：同步显示详情面板（纯文本无 IO，无需异步与缓存）
		m.prevLoading = false
		m.setPreviewContent(finderDetail(r))
		m.vp.GotoTop()
		return nil
	}
	return tea.Batch(m.renderSelectionPreview(r, m.absPath(r.Path)), m.requestBlame())
}

// renderSelectionPreview 渲染 abs 指向的文件预览（含缓存查询与异步渲染）。
func (m *Model) renderSelectionPreview(r search.Result, abs string) tea.Cmd {
	cols, rows := m.previewSize()
	proto := m.cfg.ImgProto
	jump := r.Line
	query := ""
	if m.cfg.Mode == ModeContent && !m.finder {
		query = m.input.Value() // 内容模式：预览正文内高亮命中词
	}
	if ren, ok := m.prevCache.Get(abs, cols, rows, proto, jump, query); ok {
		m.applyPreview(r.Path, ren, nil)
		return nil
	}
	render := m.renderFile // goroutine 只捕获局部变量，避免读 m 产生竞态
	cache := m.prevCache
	return func() tea.Msg {
		ren, err := render(abs, cols, rows, proto, jump, query)
		if err == nil {
			cache.Put(abs, cols, rows, proto, jump, query, ren)
		}
		return previewMsg{path: r.Path, rendered: ren, err: err}
	}
}

// finderPath finder 候选若指向真实文件则返回其路径，否则空串。
func (m *Model) finderPath(p string) string {
	if p == "" {
		return ""
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(m.root, abs)
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return abs
	}
	return ""
}

// finderDetail finder 候选行的详情面板内容。
func finderDetail(r search.Result) string {
	b := strings.Builder{}
	b.WriteString("候选行\n\n")
	b.WriteString(r.Path)
	if r.Detail != "" {
		b.WriteString("\n\n" + r.Detail)
	}
	return b.String()
}

// toggleMark 标记/取消当前选中行并下移一格（fzf 习惯）。
func (m *Model) toggleMark() tea.Cmd {
	if len(m.results) == 0 || m.sel >= len(m.results) {
		return nil
	}
	k := resultKey(m.results[m.sel])
	if m.marked[k] {
		delete(m.marked, k)
	} else {
		m.marked[k] = true
	}
	if m.sel < len(m.results)-1 {
		m.sel++
		m.adjustOffset()
		return m.followSelection()
	}
	return nil
}

// pickedOutput Enter 的输出：有标记则按当前列表顺序输出全部标记项
// （已被过滤掉的标记跳过；全部失效时退回当前选中），否则输出当前选中。
func (m *Model) pickedOutput() string {
	if len(m.marked) > 0 {
		var picked []string
		for _, r := range m.results {
			if m.marked[resultKey(r)] {
				picked = append(picked, m.pickText(r))
			}
		}
		if len(picked) > 0 {
			return strings.Join(picked, "\n")
		}
	}
	if len(m.results) > 0 && m.sel < len(m.results) {
		return m.pickText(m.results[m.sel])
	}
	return ""
}

// pickText 单条结果的输出文本：PickLine 输出原行文本（finder / 日志），
// 否则输出绝对路径。
func (m *Model) pickText(r search.Result) string {
	if m.cfg.PickLine {
		return r.Text
	}
	return m.absPath(r.Path)
}

// setPreviewContent 预览内容的统一入口：从 kitty 图形切到非图形内容时注入
// 删除序列前缀——kitty overlay 图形不占字符流，终端不会因文本替换而清除它，
// 必须显式删除，否则旧图残留在屏幕上。
func (m *Model) setPreviewContent(s string) {
	hasGfx := strings.Contains(s, "\x1b_G")
	if m.imgActive && !hasGfx {
		s = preview.KittyDeleteImage + s
	}
	m.imgActive = hasGfx
	m.vp.SetContent(s)
}

// imagePreview 图片预览不滚动：kitty overlay 不随文本滚动，sixel/halfblock
// 的占位行数已被限制在视口内，滚动只会让图形与文本错位。
func (m *Model) imagePreview() bool { return m.prevKind == string(preview.KindImage) }

// applyPreview 应用一份预览渲染结果（异步 previewMsg 与缓存命中共用）。
func (m *Model) applyPreview(path string, ren preview.Rendered, err error) {
	if path != m.prevPath {
		return // 用户已切走，丢弃
	}
	m.prevLoading = false
	if err != nil {
		m.setPreviewContent(styleErrText.Render("预览失败: " + err.Error()))
		return
	}
	m.setPreviewContent(ren.Content)
	m.prevLines = strings.Count(ren.Content, "\n") + 1
	m.prevKind = string(ren.Kind)
	m.prevLang = ren.Lang
	// 窗口化渲染时真实行号≠物理行号，滚动必须按物理位置定位
	offset := ren.JumpOffset
	if offset <= 0 {
		offset = ren.JumpLine
	}
	if offset > 0 {
		m.scrollToJump(offset, m.prevLines)
	} else {
		m.vp.GotoTop()
	}
}

// followSelectionReload 强制重新渲染当前选中项（窗口尺寸变化后调用）。
func (m *Model) followSelectionReload() tea.Cmd {
	m.prevPath = ""
	m.prevCustom = false
	return m.followSelection()
}

func (m *Model) previewSize() (cols, rows int) {
	cols = max(0, m.prevW-2)
	rows = max(0, m.panelH()-3)
	return
}

func (m *Model) scrollToJump(jump, totalLines int) {
	if jump <= 0 || totalLines <= 0 || m.vp.Height() <= 0 {
		return
	}
	y := jump - 1 - max(1, m.vp.Height()/3)
	y = max(0, y)
	y = min(y, max(0, totalLines-m.vp.Height()))
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
	return m.frame()
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
