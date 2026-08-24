package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

// Git 历史搜索模式（命令面板「Git 历史」进入）：git log -G<关键词> 流式列出
// 引入/删除了该代码的提交，列表=短hash·日期·subject，右侧详情面板显示
// commit 完整信息与涉及文件（git show --stat），Enter 复制短 hash。
// Tab 退出回到 文件/内容 模式；与既有流式消息链/取消机制完全复用。

// commitDetailMsg commit 详情回包。
type commitDetailMsg struct {
	hash    string
	content string
	err     error
}

// loadCommitDetail 异步拉取 commit 详情；cfg.GitShow 可注入 fake。
func (m *Model) loadCommitDetail(shortHash, fullHash string) tea.Cmd {
	fetch := m.cfg.GitShow
	if fetch == nil {
		fetch = func(ctx context.Context, root, hash string) (string, error) {
			return search.GitShowCommit(ctx, nil, root, hash)
		}
	}
	root := m.root
	if fullHash == "" {
		fullHash = shortHash
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := fetch(ctx, root, fullHash)
		return commitDetailMsg{hash: shortHash, content: out, err: err}
	}
}

// handleCommitDetail 详情写入预览面板（选中已切走则丢弃）。
func (m *Model) handleCommitDetail(msg commitDetailMsg) tea.Cmd {
	if msg.hash != m.prevPath {
		return nil
	}
	m.prevLoading = false
	if msg.err != nil {
		m.setPreviewContent(styleErrText.Render("commit 详情获取失败: " + msg.err.Error()))
		return nil
	}
	m.setPreviewContent(stylePanelTitle.Render("commit "+msg.hash) + "\n\n" + strings.TrimRight(msg.content, "\n"))
	m.prevKind = ""
	m.prevLang = ""
	m.prevLines = strings.Count(msg.content, "\n") + 3
	m.vp.GotoTop()
	return nil
}

// copyCommitHash gitLog 模式的 Enter：复制当前提交短 hash，不退出。
func (m *Model) copyCommitHash() tea.Cmd {
	if len(m.results) == 0 || m.sel >= len(m.results) {
		return nil
	}
	hash := m.results[m.sel].Path
	if m.writeClipboard == nil {
		m.writeClipboard = writeClipboardDefault
	}
	if err := m.writeClipboard(hash); err != nil {
		m.notice = "复制失败: " + err.Error()
		return nil
	}
	m.notice = "已复制 commit " + hash
	return nil
}

// enterGitLog 从命令面板进入 Git 历史模式。
func (m *Model) enterGitLog() tea.Cmd {
	m.gitLog = true
	m.updatePlaceholder()
	m.followKeep = ""
	return m.runSearch()
}
