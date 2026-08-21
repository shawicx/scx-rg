package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
		pathFlag    = flag.String("path", ".", "搜索根目录；配合 --follow 可指向单个日志文件")
		modeFlag    = flag.String("mode", "files", "初始模式: files | content")
		imgFlag     = flag.String("img", "auto", "图片协议: auto | kitty | sixel | none")
		debounceMs  = flag.Int("debounce-ms", 200, "搜索防抖间隔（毫秒）")
		titleFlag   = flag.String("title", "", "头部标题（如 docker:web）")
		followFlag  = flag.Bool("follow", false, "跟随 -path 指定的单个日志文件，实时刷新（tail -f 式）")
		once        = flag.Bool("once", false, "渲染一帧后退出（调试用，不进备用屏）")
		onceW       = flag.Int("w", 120, "--once 渲染宽度")
		onceH       = flag.Int("h", 40, "--once 渲染高度")
		onceQuery   = flag.String("q", "", "--once 模拟输入的搜索词")
		oncePreview = flag.String("preview-file", "", "--once 强制预览指定文件")
		versionFlag = flag.Bool("version", false, "输出版本信息并退出")
	)
	flag.Parse()

	if *versionFlag {
		fmt.Printf("scx-rg %s (commit %s, built %s, %s/%s)\n", version, commit, date, runtime.GOOS, runtime.GOARCH)
		return
	}

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
		Debounce:    time.Duration(*debounceMs) * time.Millisecond,
		ImgProto:    proto,
		RgAvailable: search.RgAvailable(),
		Title:       *titleFlag,
	}

	if *followFlag {
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
	} else {
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
		fmt.Println(m.RenderOnce(*onceW, *onceH, *onceQuery, *oncePreview))
		return
	}

	// IDE 运行窗/管道等环境没有控制终端，bubbletea 拿不到 TTY 会直接报错退出；
	// 此时降级为单帧渲染，而不是崩溃。
	if !hasTTY() {
		fmt.Fprintln(os.Stderr, "⚠ 当前环境没有可用的 TTY（IDE 运行窗/管道），已降级为单帧渲染；请在真实终端中运行获得交互体验")
		fmt.Println(m.RenderOnce(*onceW, *onceH, *onceQuery, *oncePreview))
		return
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		die(err)
	}
	printPicked(final)
}

// runLogSource docker/k8s 子命令：默认持续跟随新日志（tail -f 式，
// 初始内容与一次性快照相同且实时更新）；--snapshot 退回一次性快照。
func runLogSource(kind string, args []string) {
	target := logs.Target{Kind: kind}
	if kind == "k8s" {
		target.Kind = "kubectl"
	}
	var snapshot, legacyFollow bool
	fs := flag.NewFlagSet(kind, flag.ContinueOnError)
	fs.BoolVar(&snapshot, "snapshot", false, "只抓取一次快照，不跟随（默认跟随）")
	fs.BoolVar(&legacyFollow, "follow", false, "（默认已跟随，兼容保留）")
	fs.BoolVar(&legacyFollow, "f", false, "（默认已跟随，兼容保留）")
	fs.StringVar(&target.Namespace, "n", "", "namespace（k8s）")
	fs.StringVar(&target.Container, "c", "", "指定容器（k8s 多容器 Pod）")
	_ = fs.Parse(args)
	follow := legacyFollow || !snapshot // 日志是活数据：默认跟随，避免「搜不到最新日志」

	if !target.Available() {
		die(fmt.Errorf("未找到 %s 命令", target.Bin()))
	}
	if !search.RgAvailable() {
		die(errors.New("日志检索需要 ripgrep（brew install ripgrep）"))
	}

	dir, err := os.MkdirTemp("", "scx-rg-log-")
	if err != nil {
		die(err)
	}
	defer os.RemoveAll(dir)

	// 无参数：进入源选择器（免记忆），选中后按上述默认跟随/快照。
	rest := fs.Args()
	if len(rest) == 0 {
		m := tui.New(tui.Config{
			PickerKind:  target.Kind,
			SnapshotDir: dir,
			FollowPick:  follow,
			LogTail:     logTail,
			Mode:        tui.ModeContent,
			ImgProto:    preview.ProtocolNone,
			RgAvailable: true,
			PickLine:    true,
		})
		p := tea.NewProgram(m, tea.WithAltScreen())
		final, err := p.Run()
		if err != nil {
			die(err)
		}
		printPicked(final)
		return
	}
	target.Name = rest[0]
	logPath := filepath.Join(dir, kind+".log")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if follow {
		fmt.Fprintf(os.Stderr, "正在跟随 %s %s 的日志（初始 tail %d 行，实时更新，Ctrl+C 退出）…\n",
			kind, target.Name, logTail)
		if err := logs.Follow(ctx, target, logTail, logPath); err != nil {
			die(err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "正在抓取 %s %s 最近 %d 行日志快照…\n", kind, target.Name, logTail)
		snap, err := logs.Snapshot(ctx, nil, target, logTail)
		if err != nil {
			die(err)
		}
		if err := os.Rename(snap, logPath); err != nil {
			_ = os.Remove(snap)
			die(err)
		}
	}

	cfg := tui.Config{
		Root:        dir,
		Mode:        tui.ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
		Title:       kind + ":" + target.Name,
		PickLine:    true, // Enter 输出选中日志行（快照文件退出即删）
	}
	if follow {
		cfg.FollowFile = logPath
	}
	m := tui.New(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
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
