package tui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

// Blame 状态栏摘要：显示当前选中行的「短hash 作者 相对时间」（如
// abc1234 john 3d）。整文件 blame 一次拉取，按 文件+mtime 缓存（LRU 容量
// 32 文件），选中切换命中缓存零开销；未命中异步拉取不阻塞按键。
// 非 git 仓库 / git 不可用时静默隐藏。Ctrl+B 即时开关。

const blameCacheCap = 32

// blameEntry 一个文件的整份行级 blame。
type blameEntry struct {
	mtime int64
	lines map[int]string // 行号(1 起) → 摘要
}

// blameCache LRU：按路径存 blameEntry，容量 32。
type blameCache struct {
	entries map[string]blameEntry
	order   []string // 最旧在前
}

func newBlameCache() *blameCache {
	return &blameCache{entries: map[string]blameEntry{}}
}

func (c *blameCache) get(path string, mtime int64) (map[int]string, bool) {
	e, ok := c.entries[path]
	if !ok || e.mtime != mtime {
		return nil, false
	}
	// 触碰 LRU 顺序
	for i, p := range c.order {
		if p == path {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	c.order = append(c.order, path)
	return e.lines, true
}

func (c *blameCache) put(path string, mtime int64, lines map[int]string) {
	if _, ok := c.entries[path]; !ok {
		c.order = append(c.order, path)
	}
	c.entries[path] = blameEntry{mtime: mtime, lines: lines}
	for len(c.order) > blameCacheCap {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

// blameMsg blame 回包。
type blameMsg struct {
	key     string // path:line 判废用
	lines   map[int]string
	summary string // 命中行的摘要（lines 已含）
	err     error
}

// requestBlame 为选中行请求 blame 摘要：命中缓存同步生效，否则异步拉取。
// 配置关 / 非文件选中项直接跳过。
func (m *Model) requestBlame() tea.Cmd {
	if !m.blameOn || len(m.results) == 0 || m.sel >= len(m.results) {
		m.blameText = ""
		return nil
	}
	if m.finder || m.gitLog {
		m.blameText = ""
		return nil
	}
	r := m.results[m.sel]
	if r.Path == "" || strings.HasPrefix(r.Path, "..") {
		m.blameText = ""
		return nil
	}
	// 额外目录（绝对路径）不在主仓库根下时跳过 blame
	if filepath.IsAbs(r.Path) && !strings.HasPrefix(r.Path, m.root+string(filepath.Separator)) {
		m.blameText = ""
		return nil
	}
	line := max(1, r.Line)
	key := r.Path + ":" + strconv.Itoa(line)
	st, err := statFile(m.absPath(r.Path))
	if err != nil {
		m.blameText = ""
		return nil
	}
	if lines, ok := m.blameCache.get(r.Path, st); ok {
		m.blameText = lines[line]
		if m.blameText == "" {
			m.blameText = lines[1] // 行号越界（文件变短）回退首行
		}
		return nil
	}
	m.blameActive = key
	fetch := m.cfg.BlameFetch
	if fetch == nil {
		fetch = func(ctx context.Context, root, rel string) (string, error) {
			return search.GitBlameFile(ctx, nil, root, rel)
		}
	}
	root, rel := m.root, r.Path
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		raw, err := fetch(ctx, root, rel)
		if err != nil {
			return blameMsg{key: key, err: err}
		}
		lines := search.ParseBlamePorcelain(raw)
		return blameMsg{key: key, lines: lines, summary: lines[line]}
	}
}

// handleBlame 应用 blame 回包：缓存整份，选中项未变则显示摘要。
func (m *Model) handleBlame(msg blameMsg) tea.Cmd {
	m.blameActive = ""
	if msg.err != nil || msg.lines == nil {
		return nil // 静默：非 git 仓库是常态
	}
	r := m.results[m.sel]
	st, err := statFile(m.absPath(r.Path))
	if err == nil {
		m.blameCache.put(r.Path, st, msg.lines)
	}
	if len(m.results) > 0 && m.sel < len(m.results) {
		key := r.Path + ":" + strconv.Itoa(max(1, r.Line))
		if key == msg.key {
			m.blameText = msg.summary
			if m.blameText == "" {
				m.blameText = msg.lines[1]
			}
		}
	}
	return nil
}

// statFile 取文件 mtime（Unix 秒）；blameCache 的失效键。
var statFile = func(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.ModTime().Unix(), nil
}
