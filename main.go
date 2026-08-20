package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/logs"
	"scx-rg/internal/preview"
	"scx-rg/internal/search"
	"scx-rg/internal/tui"
)

// dockerTail 抓取容器日志的行数上限。
const dockerTail = 100000

func main() {
	if len(os.Args) > 1 && os.Args[1] == "docker" {
		runDocker(os.Args[2:])
		return
	}
	var (
		pathFlag    = flag.String("path", ".", "搜索根目录")
		modeFlag    = flag.String("mode", "files", "初始模式: files | content")
		imgFlag     = flag.String("img", "auto", "图片协议: auto | kitty | sixel | none")
		debounceMs  = flag.Int("debounce-ms", 200, "搜索防抖间隔（毫秒）")
		titleFlag   = flag.String("title", "", "头部标题（如 docker:web）")
		once        = flag.Bool("once", false, "渲染一帧后退出（调试用，不进备用屏）")
		onceW       = flag.Int("w", 120, "--once 渲染宽度")
		onceH       = flag.Int("h", 40, "--once 渲染高度")
		onceQuery   = flag.String("q", "", "--once 模拟输入的搜索词")
		oncePreview = flag.String("preview-file", "", "--once 强制预览指定文件")
	)
	flag.Parse()

	root, err := filepath.Abs(*pathFlag)
	if err != nil {
		die(err)
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		die(fmt.Errorf("不是有效目录: %s", root))
	}

	proto := preview.ParseProtocol(*imgFlag)
	if proto == preview.ProtocolAuto {
		proto = preview.Detect()
	}

	mode := tui.ModeFiles
	if *modeFlag == "content" {
		mode = tui.ModeContent
	}

	m := tui.New(tui.Config{
		Root:        root,
		Mode:        mode,
		Debounce:    time.Duration(*debounceMs) * time.Millisecond,
		ImgProto:    proto,
		RgAvailable: search.RgAvailable(),
		Title:       *titleFlag,
	})

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

// runDocker `scx-rg docker <容器名>`：抓取容器日志快照后进入全文检索。
func runDocker(args []string) {
	if !logs.DockerAvailable() {
		die(errors.New("未找到 docker 命令"))
	}
	if !search.RgAvailable() {
		die(errors.New("日志检索需要 ripgrep（brew install ripgrep）"))
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法: scx-rg docker <容器名>    容器列表：")
		cmd := exec.Command("docker", "ps", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		_ = cmd.Run()
		os.Exit(1)
	}
	container := args[0]

	fmt.Fprintf(os.Stderr, "正在抓取 %s 最近 %d 行日志…\n", container, dockerTail)
	dir, err := os.MkdirTemp("", "scx-rg-docker-")
	if err != nil {
		die(err)
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	snap, err := logs.SnapshotDocker(ctx, logs.DefaultRunner, container, dockerTail)
	cancel()
	if err != nil {
		die(err)
	}
	if err := os.Rename(snap, filepath.Join(dir, "docker.log")); err != nil {
		_ = os.Remove(snap)
		die(err)
	}

	m := tui.New(tui.Config{
		Root:        dir,
		Mode:        tui.ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
		Title:       "docker:" + container,
		PickLine:    true, // Enter 输出选中日志行（快照文件退出即删）
	})
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
