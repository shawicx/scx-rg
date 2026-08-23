package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

// Git 筛选（Ctrl+T 筛选栏第三段，仅 git 仓库内出现）：
// 全部 / 仅改动（对 HEAD 的 tracked 变更）/ 仅暂存。
// files 模式在枚举层过滤（FilesProvider.Allow，rg 与 walk 单点覆盖）；
// content 模式在客户端按路径过滤（resultPasses，流式与 refilter 共用）。
// 未跟踪的新文件不属于 git diff，不在筛选范围内（与 git 语义一致）。

// rangeGitPresets Git 段预设。
var rangeGitPresets = []struct {
	label string
	n     int
}{
	{"全部", 0},
	{"仅改动", 1},
	{"仅暂存", 2},
}

func gitPresetIndex(n int) int {
	for i, p := range rangeGitPresets {
		if p.n == n {
			return i
		}
	}
	return 0
}

// gitFilesMsg Git 文件集回包（探测与筛选共用；err 非空 = 非 git 仓库或 git 不可用）。
type gitFilesMsg struct {
	staged bool
	files  []string
	err    error
}

// loadGitFiles 异步拉取变更文件集；cfg.GitFiles 可注入 fake。
func (m *Model) loadGitFiles(staged bool) tea.Cmd {
	fetch := m.cfg.GitFiles
	if fetch == nil {
		fetch = func(ctx context.Context, root string, staged bool) ([]string, error) {
			return search.GitChangedFiles(ctx, nil, root, staged)
		}
	}
	root := m.root
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		files, err := fetch(ctx, root, staged)
		return gitFilesMsg{staged: staged, files: files, err: err}
	}
}

// handleGitFiles 应用文件集回包：探测成功置 gitOK（第三段可见）；筛选生效
// 后按模式重跑——files 重枚举，content 本地重过滤。
func (m *Model) handleGitFiles(msg gitFilesMsg) tea.Cmd {
	m.gitLoading = false
	m.gitKnown = true
	if msg.err != nil {
		m.gitOK = false
		if m.gitFilter > 0 {
			// 用户已选 Git 筛选但拿不到文件集：回退「全部」并提示
			m.gitFilter = 0
			m.rangeSel[2] = 0
			m.gitAllow = nil
			m.notice = "Git 文件集获取失败: " + msg.err.Error()
		}
		return nil
	}
	m.gitOK = true
	if m.gitFilter == 0 {
		m.gitAllow = nil
		return nil // 仅探测，筛选未变化
	}
	allow := make(map[string]bool, len(msg.files))
	for _, f := range msg.files {
		allow[f] = true
	}
	m.gitAllow = allow
	return m.reapplyGitFilter()
}

// applyGitFilter 响应 Git 段 chip 切换：全部=清空文件集就地重跑；
// 仅改动/仅暂存=异步拉取，回包后由 handleGitFiles 应用。
func (m *Model) applyGitFilter(n int) tea.Cmd {
	m.gitFilter = n
	if n == 0 {
		m.gitAllow = nil
		return m.reapplyGitFilter()
	}
	m.gitLoading = true
	return m.loadGitFiles(n == 2)
}

// reapplyGitFilter 把当前 Git 筛选应用到结果：files 模式重新枚举，
// content / 全文回退在客户端重过滤（raw 缓冲仍持有全量）。
func (m *Model) reapplyGitFilter() tea.Cmd {
	if m.mode == ModeContent || m.fallbackActive {
		return m.refilter(true)
	}
	return m.runSearch()
}

// gitAllowsPath 结果路径是否通过 Git 筛选（路径相对搜索根，与文件集同口径）。
func (m *Model) gitAllowsPath(p string) bool {
	return len(m.gitAllow) == 0 || m.gitAllow[p]
}
