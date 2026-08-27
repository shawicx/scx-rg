package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

// ast-grep 替换：R（空输入）或命令面板进入。两段输入（AST 模式 → 重写
// 模板）后扫描；匹配列表复用结果面板（path:line + 命中文本），右侧预览
// 显示 -旧/+新 diff；y 应用当前并前进、a 应用全部、n 跳过、Esc 退出。
// 安全模型：进入扫描前要求 git 仓库且工作区干净（改动审查/回滚交给 git），
// 应用由 search.ApplyAstMatches 完成（字节区间拼接）。
// astMode 是模态：除导航/y/n/a/Esc/Ctrl+C 外的按键退出该模式。

// astScanMsg 扫描回包。
type astScanMsg struct {
	matches []search.AstMatch
	err     error
}

// astAppliedMsg 应用完成。
type astAppliedMsg struct {
	count int // 应用的匹配数
	files int
	err   error
}

// astGrepAvailable 探测 ast-grep 二进制；测试可注入。
var astGrepAvailable = search.AstGrepAvailable

// enterReplace 打开替换输入浮层（无 ast-grep / 不适用模式给出提示）。
func (m *Model) enterReplace() tea.Cmd {
	if !astGrepAvailable() {
		m.notice = "未找到 ast-grep（https://ast-grep.com，brew install ast-grep）"
		return nil
	}
	if m.finder || m.picking || m.gitLog {
		m.notice = "当前模式不支持 AST 替换"
		return nil
	}
	if len(m.extraRoots) > 0 {
		m.notice = "多目录 workspace 暂不支持 AST 替换"
		return nil
	}
	m.replaceOpen = true
	m.replaceStage = 0
	m.replacePattern = ""
	m.replaceRewrite = ""
	return nil
}

// handleReplaceKey 两段输入浮层：第一段 Enter 检查工作区干净后进入第二段，
// 第二段 Enter 发起扫描。
func (m *Model) handleReplaceKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		m.picked = ""
		return m, tea.Quit

	case "esc":
		m.replaceOpen = false
		m.replacePattern, m.replaceRewrite = "", ""
		return m, nil

	case "tab":
		if m.replaceStage == 0 && strings.TrimSpace(m.replacePattern) != "" {
			m.replaceStage = 1
			return m, nil
		}

	case "enter":
		if m.replaceStage == 0 {
			if strings.TrimSpace(m.replacePattern) == "" {
				m.notice = "AST 模式不能为空"
				return m, nil
			}
			m.replaceStage = 1
			return m, nil
		}
		return m, m.startAstScan()

	case "backspace":
		if m.replaceStage == 0 {
			r := []rune(m.replacePattern)
			if len(r) > 0 {
				m.replacePattern = string(r[:len(r)-1])
			}
		} else {
			r := []rune(m.replaceRewrite)
			if len(r) > 0 {
				m.replaceRewrite = string(r[:len(r)-1])
			}
		}
		return m, nil
	}
	if msg.Text != "" && msg.Code != tea.KeyExtended {
		if m.replaceStage == 0 {
			m.replacePattern += msg.Text
		} else {
			m.replaceRewrite += msg.Text
		}
	}
	return m, nil
}

// startAstScan 工作区干净检查 + 异步扫描。
func (m *Model) startAstScan() tea.Cmd {
	clean := m.cfg.GitClean
	if clean == nil {
		clean = func(ctx context.Context, root string) (bool, error) {
			return search.GitWorktreeClean(ctx, nil, root)
		}
	}
	root, pattern, rewrite := m.root, m.replacePattern, m.replaceRewrite
	scan := m.cfg.AstScan
	if scan == nil {
		scan = func(ctx context.Context, root, pattern, rewrite string) ([]search.AstMatch, error) {
			return search.AstGrepScan(ctx, nil, root, pattern, rewrite)
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ok, err := clean(ctx, root)
		if err != nil {
			return astScanMsg{err: fmt.Errorf("git 状态检查失败: %w", err)}
		}
		if !ok {
			return astScanMsg{err: fmt.Errorf("工作区不干净：AST 替换要求干净工作区（改动审查与回滚交给 git），请先 commit 或 stash")}
		}
		matches, err := scan(ctx, root, pattern, rewrite)
		return astScanMsg{matches: matches, err: err}
	}
}

// handleAstScan 应用扫描结果：非空进入匹配列表模式。
func (m *Model) handleAstScan(msg astScanMsg) tea.Cmd {
	m.replaceOpen = false
	if msg.err != nil {
		m.notice = msg.err.Error()
		return nil
	}
	if len(msg.matches) == 0 {
		m.notice = "无匹配（检查 AST 模式）"
		return nil
	}
	m.astMode = true
	m.astMatches = msg.matches
	m.results = astResults(msg.matches, m.root)
	m.sel, m.offset = 0, 0
	m.adjustOffset()
	return m.showAstDiff()
}

// astMatches 的结果视图：Path 相对主根显示，Detail 携带重写文本。
func astResults(matches []search.AstMatch, root string) []search.Result {
	out := make([]search.Result, len(matches))
	for i, mm := range matches {
		p := mm.File
		if rel, err := filepath.Rel(root, mm.File); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
		out[i] = search.Result{Path: p, Line: mm.Line, Text: strings.SplitN(mm.Text, "\n", 2)[0], Detail: mm.Replacement}
	}
	return out
}

// handleAstKey 匹配列表模式按键：y 应用当前、a 应用全部、n 跳过，
// 导航更新 diff 预览，其余任意键退出。
func (m *Model) handleAstKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		m.picked = ""
		return m, tea.Quit

	case "y":
		if m.sel < len(m.astMatches) {
			return m, m.applyAst([]search.AstMatch{m.astMatches[m.sel]})
		}
		return m, nil

	case "a":
		if len(m.astMatches) > 0 {
			return m, m.applyAst(m.astMatches)
		}
		return m, nil

	case "n", "down", "ctrl+n":
		if m.sel < len(m.results)-1 {
			m.sel++
			m.adjustOffset()
		}
		return m, m.showAstDiff()

	case "up", "ctrl+p":
		if m.sel > 0 {
			m.sel--
			m.adjustOffset()
		}
		return m, m.showAstDiff()

	case "esc":
		m.exitAstMode()
		m.notice = "已退出替换（已应用的改动保留，git diff 审查）"
		return m, nil
	}
	m.exitAstMode()
	return m, nil
}

func (m *Model) exitAstMode() {
	m.astMode = false
	m.astMatches = nil
}

// applyAst 应用若干匹配：落盘后从列表移除并更新预览。
func (m *Model) applyAst(matches []search.AstMatch) tea.Cmd {
	apply := m.cfg.AstApply
	if apply == nil {
		apply = func(_ string, ms []search.AstMatch) (int, error) { return search.ApplyAstMatches(ms) }
	}
	pending := matches
	root := m.root
	return func() tea.Msg {
		files, err := apply(root, pending)
		if err != nil {
			return astAppliedMsg{err: err}
		}
		return astAppliedMsg{count: len(pending), files: files}
	}
}

// handleAstApplied 应用回包：从匹配列表移除已应用项。
func (m *Model) handleAstApplied(msg astAppliedMsg) tea.Cmd {
	if msg.err != nil {
		m.notice = "应用失败: " + msg.err.Error()
		return nil
	}
	m.notice = fmt.Sprintf("已应用 %d 处（%d 个文件）· git diff 审查 / git checkout -- . 回滚", msg.count, msg.files)
	// 从列表移除已应用项（applyAst 单条时是当前选中；全部时清空）
	if len(m.astMatches) == msg.count {
		m.astMatches = nil
		m.results = nil
		m.exitAstMode()
		m.setPreviewContent("")
		m.prevPath = ""
		return nil
	}
	if m.sel < len(m.astMatches) {
		m.astMatches = append(m.astMatches[:m.sel], m.astMatches[m.sel+1:]...)
		m.results = astResults(m.astMatches, m.root)
		if m.sel >= len(m.results) && m.sel > 0 {
			m.sel--
		}
		m.adjustOffset()
	}
	return m.showAstDiff()
}

// showAstDiff 当前选中匹配的 -旧/+新 预览。
func (m *Model) showAstDiff() tea.Cmd {
	if m.sel >= len(m.astMatches) {
		return nil
	}
	mm := m.astMatches[m.sel]
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render("AST 替换预览") + styleDim.Render(fmt.Sprintf("  %d/%d", m.sel+1, len(m.astMatches))) + "\n\n")
	for _, l := range strings.Split(mm.Text, "\n") {
		b.WriteString(styleErrText.Render("- "+l) + "\n")
	}
	for _, l := range strings.Split(mm.Replacement, "\n") {
		b.WriteString(styleRowMarker.Render("+ "+l) + "\n")
	}
	b.WriteString("\n" + styleDim.Render("y 应用当前 · a 应用全部 · n 跳过 · Esc 退出") + "\n")
	m.prevPath = mm.File
	m.prevCustom = true
	m.prevKind = ""
	m.setPreviewContent(strings.TrimRight(b.String(), "\n"))
	m.vp.GotoTop()
	return nil
}

// replaceView 两段输入浮层。
func (m *Model) replaceView() string {
	title := stylePanelTitle.Render("AST 替换（ast-grep）") + styleDim.Render("  安全：要求干净 git 工作区")
	prompt1 := styleMatch.Render("模式  > ") + m.replacePattern
	prompt2 := styleMatch.Render("重写  > ") + m.replaceRewrite
	lines := []string{title, "", prompt1}
	if m.replaceStage >= 1 {
		lines = append(lines, "", prompt2)
	}
	lines = append(lines, "",
		stylePlaceholder.Render("  模式用 $VAR 元变量（如 function $F($$$A) { $$$B }）"),
		stylePlaceholder.Render("  重写模板复用同名元变量（如 fn $F($$$A) { $$$B }）"),
		"",
		styleDim.Render("  Tab/Enter 下一段 · Esc 取消"))
	avail := max(0, m.panelH()-2)
	for len(lines) < avail {
		lines = append(lines, "")
	}
	if len(lines) > avail {
		lines = lines[:avail]
	}
	return styleBorderIdle.Width(m.frameW()).Render(strings.Join(lines, "\n"))
}
