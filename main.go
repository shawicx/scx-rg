package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"scx-rg/internal/config"
	"scx-rg/internal/logs"
	"scx-rg/internal/preview"
	"scx-rg/internal/search"
	"scx-rg/internal/tui"
)

// logTail 抓取日志的行数上限。
const logTail = 100000

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "docker" || os.Args[1] == "k8s") {
		runLogSource(os.Args[1], os.Args[2:])
		return
	}
	var (
		pathFlag     = flag.String("path", ".", "搜索根目录；配合 --follow 可指向单个日志文件")
		modeFlag     = flag.String("mode", "files", "初始模式: files | content")
		imgFlag      = flag.String("img", "auto", "图片协议: auto | kitty | sixel | halfblock | none")
		providerFlag = flag.String("provider", "", "候选来源: stdin | docker-ps（管道候选取代文件搜索，Enter 输出选中行）")
		debounceMs   = flag.Int("debounce-ms", 200, "搜索防抖间隔（毫秒）")
		titleFlag    = flag.String("title", "", "头部标题（如 docker:web）")
		followFlag   = flag.Bool("follow", false, "跟随 -path 指定的单个日志文件，实时刷新（tail -f 式）")
		once         = flag.Bool("once", false, "渲染一帧后退出（调试用，不进备用屏）")
		onceW        = flag.Int("w", 120, "--once 渲染宽度")
		onceH        = flag.Int("h", 40, "--once 渲染高度")
		onceQuery    = flag.String("q", "", "--once 模拟输入的搜索词")
		oncePreview  = flag.String("preview-file", "", "--once 强制预览指定文件")
		versionFlag  = flag.Bool("version", false, "输出版本信息并退出")
	)
	flag.Parse()

	if *versionFlag {
		fmt.Printf("scx-rg %s (commit %s, built %s, %s/%s)\n", version, commit, date, runtime.GOOS, runtime.GOARCH)
		return
	}

	// 配置文件 ~/.config/scx-rg/config.toml：未配置/损坏回退默认，不阻断启动。
	// 优先级：flag 显式设置 > config.toml > 内置默认。
	userCfg := config.Load("")
	debounce := time.Duration(userCfg.DebounceMS) * time.Millisecond
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "debounce-ms" {
			debounce = time.Duration(*debounceMs) * time.Millisecond
		}
	})
	tui.ApplyTheme(userCfg.Theme.Preset, userCfg.Theme.Accent, userCfg.Theme.Match, userCfg.Theme.RowMarker)

	proto := preview.ParseProtocol(*imgFlag)
	if proto == preview.ProtocolAuto {
		proto = preview.Detect()
	}

	mode := tui.ModeFiles
	if *modeFlag == "content" {
		mode = tui.ModeContent
	}

	cfg := tui.Config{
		Mode:        mode,
		Debounce:    debounce,
		ImgProto:    proto,
		RgAvailable: search.RgAvailable(),
		Title:       *titleFlag,
		IgnoreDirs:  userCfg.Ignore,

		EditorCommand: userCfg.Editor.Command,
		EditorArgs:    userCfg.Editor.Args,

		HistorySize: userCfg.History.Size,
		ShowBlame:   userCfg.Git.ShowBlame == nil || *userCfg.Git.ShowBlame,
	}

	switch {
	case *providerFlag != "":
		cands, name, err := loadCandidates(*providerFlag)
		if err != nil {
			die(err)
		}
		cfg.Candidates = cands
		cfg.FinderName = name
		cfg.PickLine = true // Enter 输出原行文本
		cfg.Title = name

	case *followFlag:
		p := *pathFlag
		if rest := flag.Args(); len(rest) > 0 {
			p = rest[0] // 支持 --follow /var/log/app.log 的直觉写法
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			die(err)
		}
		st, err := os.Stat(abs)
		if err != nil || st.IsDir() {
			die(errors.New("--follow 需要指向一个具体的日志文件，如 --follow /var/log/app.log"))
		}
		cfg.Root = filepath.Dir(abs)
		cfg.FollowFile = abs
		cfg.Title = filepath.Base(abs)
		cfg.PickLine = true
		cfg.Mode = tui.ModeContent
	default:
		root, err := filepath.Abs(*pathFlag)
		if err != nil {
			die(err)
		}
		st, err := os.Stat(root)
		if err != nil {
			die(fmt.Errorf("路径不存在: %s", root))
		}
		if !st.IsDir() {
			die(fmt.Errorf("%s 是文件；如需实时跟随请加 --follow", root))
		}
		cfg.Root = root
	}

	m := tui.New(cfg)

	if *once {
		_, _ = lipgloss.Println(m.RenderOnce(*onceW, *onceH, *onceQuery, *oncePreview))
		return
	}

	// IDE 运行窗/管道等环境没有控制终端，bubbletea 拿不到 TTY 会直接报错退出；
	// 此时降级为单帧渲染，而不是崩溃。
	if !hasTTY() {
		fmt.Fprintln(os.Stderr, "⚠ 当前环境没有可用的 TTY（IDE 运行窗/管道），已降级为单帧渲染；请在真实终端中运行获得交互体验")
		_, _ = lipgloss.Println(m.RenderOnce(*onceW, *onceH, *onceQuery, *oncePreview))
		return
	}

	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		die(err)
	}
	clearKittyGraphics(proto)
	printPicked(final)
}

// clearKittyGraphics 退出前清空终端内全部 kitty 图形：alt-screen 退出后
// overlay 图形默认保留，不清会在终端里残留到清屏。
func clearKittyGraphics(proto preview.Protocol) {
	if proto == preview.ProtocolKitty {
		fmt.Fprint(os.Stdout, preview.KittyDeleteAll)
	}
}

// loadCandidates 解析 --provider 预设数据源为静态候选列表。
func loadCandidates(name string) ([]search.Candidate, string, error) {
	switch name {
	case "stdin":
		cands, err := readStdinCandidates()
		if err != nil {
			return nil, "", err
		}
		return cands, "stdin", nil
	case "docker-ps":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srcs, err := logs.ListSources(ctx, nil, "docker")
		if err != nil {
			return nil, "", fmt.Errorf("docker 容器列表获取失败: %w", err)
		}
		cands := make([]search.Candidate, 0, len(srcs))
		for _, s := range srcs {
			detail := s.Detail
			if s.Status != "" {
				detail = strings.TrimSpace(detail + " · " + s.Status)
			}
			cands = append(cands, search.Candidate{Text: s.Target.Name, Detail: detail})
		}
		return cands, "docker", nil
	}
	return nil, "", fmt.Errorf("未知 --provider %q（支持: stdin | docker-ps）", name)
}

// readStdinCandidates 读管道输入的候选行（每行一条，跳过空行）。
func readStdinCandidates() ([]search.Candidate, error) {
	if term.IsTerminal(os.Stdin.Fd()) {
		return nil, errors.New("--provider stdin 需要管道输入，如: fd --type f | scx-rg --provider stdin")
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var out []search.Candidate
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, search.Candidate{Text: line})
		if len(out) >= 100000 {
			break // 防御性上限：与日志 tail 同量级
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("读取 stdin 失败: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("stdin 无候选行")
	}
	return out, nil
}

// runLogSource docker/k8s 子命令：默认实时多面板日志（tee 落盘缓存目录，
// 默认 scx-rg 命令随时可搜）；--snapshot 退回一次性快照+检索（旧行为）。
func runLogSource(kind string, args []string) {
	target := logs.Target{Kind: kind}
	if kind == "k8s" {
		target.Kind = "kubectl"
	}
	var snapshot, legacyFollow bool
	fs := flag.NewFlagSet(kind, flag.ContinueOnError)
	fs.BoolVar(&snapshot, "snapshot", false, "一次性快照后进入检索（默认实时多面板）")
	fs.BoolVar(&legacyFollow, "follow", false, "（默认已实时，兼容保留）")
	fs.BoolVar(&legacyFollow, "f", false, "（默认已实时，兼容保留）")
	fs.StringVar(&target.Namespace, "n", "", "namespace（k8s）")
	fs.StringVar(&target.Container, "c", "", "指定容器（k8s 多容器 Pod）")
	_ = fs.Parse(args)
	rest := fs.Args()

	if !target.Available() {
		die(fmt.Errorf("未找到 %s 命令", target.Bin()))
	}

	if snapshot {
		runSnapshotSession(kind, target, rest) // 旧流程：需要 rg
		return
	}

	// 实时模式：tee 落盘 os.UserCacheDir()/scx-rg/logs（稳定路径，
	// 默认命令/–-follow 可直接检索）；实时视图本身不依赖 rg。
	cache, err := os.UserCacheDir()
	if err != nil {
		die(err)
	}
	liveDir := filepath.Join(cache, "scx-rg", "logs")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		die(err)
	}

	cfg := tui.Config{
		PickerKind:  target.Kind,
		LiveDir:     liveDir,
		LogTail:     logTail,
		Mode:        tui.ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: search.RgAvailable(),
	}
	if len(rest) > 0 {
		target.Name = rest[0]
		cfg.LiveTargets = []logs.Target{target} // 跳过选择器直达单面板
		fmt.Fprintf(os.Stderr, "实时日志 %s %s（落盘 %s，Ctrl+C 退出）…\n",
			kind, target.Name, logs.LivePath(liveDir, target))
	} else {
		cfg.LivePick = true // 选择器 Tab 多选 ≤4，Enter 进实时
	}
	p := tea.NewProgram(tui.New(cfg))
	final, err := p.Run()
	if err != nil {
		die(err)
	}
	printPicked(final)
}

// runSnapshotSession --snapshot 旧流程：临时目录快照 + 既有检索界面。
// 快照文件随会话退出删除，行为与分离改造前一致。
func runSnapshotSession(kind string, target logs.Target, rest []string) {
	if !search.RgAvailable() {
		die(errors.New("日志检索需要 ripgrep（brew install ripgrep）"))
	}
	dir, err := os.MkdirTemp("", "scx-rg-log-")
	if err != nil {
		die(err)
	}
	defer os.RemoveAll(dir)

	if len(rest) == 0 {
		m := tui.New(tui.Config{
			PickerKind:  target.Kind,
			SnapshotDir: dir,
			LogTail:     logTail,
			Mode:        tui.ModeContent,
			ImgProto:    preview.ProtocolNone,
			RgAvailable: true,
			PickLine:    true,
		})
		p := tea.NewProgram(m)
		final, err := p.Run()
		if err != nil {
			die(err)
		}
		printPicked(final)
		return
	}
	target.Name = rest[0]
	fmt.Fprintf(os.Stderr, "正在抓取 %s %s 最近 %d 行日志快照…\n", kind, target.Name, logTail)
	snap, err := logs.Snapshot(context.Background(), nil, target, logTail)
	if err != nil {
		die(err)
	}
	logPath := filepath.Join(dir, kind+".log")
	if err := os.Rename(snap, logPath); err != nil {
		_ = os.Remove(snap)
		die(err)
	}
	m := tui.New(tui.Config{
		Root:        dir,
		Mode:        tui.ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
		Title:       kind + ":" + target.Name,
		PickLine:    true, // Enter 输出选中日志行（快照文件退出即删）
	})
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		die(err)
	}
	printPicked(final)
}

func printPicked(final tea.Model) {
	if fm, ok := final.(*tui.Model); ok {
		if picked := fm.PickedPath(); picked != "" {
			fmt.Println(picked)
		}
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "scx-rg:", err)
	os.Exit(1)
}

// hasTTY 探测是否存在可用的控制终端。
func hasTTY() bool {
	if runtime.GOOS == "windows" {
		return true // 交给 bubbletea 自行处理
	}
	f, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
