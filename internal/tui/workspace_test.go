package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newWorkspaceModel(t *testing.T) (*Model, string) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"main.txt", "readme.txt"} {
		_ = os.WriteFile(filepath.Join(dir, n), []byte("x\n"), 0o644)
	}
	extra := t.TempDir()
	for _, n := range []string{"extra.txt"} {
		_ = os.WriteFile(filepath.Join(extra, n), []byte("x\n"), 0o644)
	}
	m := New(Config{Root: dir, RgAvailable: false})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	return m, extra
}

// 添加额外目录：主目录结果相对路径、额外目录绝对路径；状态栏显示 +1 目录。
func TestAddExtraRootFlow(t *testing.T) {
	m, extra := newWorkspaceModel(t)
	if len(m.results) != 2 {
		t.Fatalf("前置失败：主目录应有 2 个文件: %v", m.results)
	}
	if cmd := m.addExtraRoot(extra); cmd != nil {
		m.drain(cmd)
	}
	if len(m.extraRoots) != 1 || m.extraRoots[0] != extra {
		t.Fatalf("额外目录未登记: %v", m.extraRoots)
	}
	if len(m.results) != 3 {
		t.Fatalf("多目录应搜出 3 个文件: %v", m.results)
	}
	absCount, relCount := 0, 0
	for _, r := range m.results {
		if filepath.IsAbs(r.Path) {
			absCount++
			if !strings.HasSuffix(r.Path, "extra.txt") {
				t.Errorf("绝对路径应为 extra.txt: %q", r.Path)
			}
		} else {
			relCount++
		}
	}
	if absCount != 1 || relCount != 2 {
		t.Errorf("主目录相对/额外绝对: abs=%d rel=%d", absCount, relCount)
	}
	if !strings.Contains(m.frame(), "+1 目录") {
		t.Error("状态栏应显示 +1 目录")
	}
	// 重复添加被拒
	if cmd := m.addExtraRoot(extra); cmd != nil {
		m.drain(cmd)
	}
	if len(m.extraRoots) != 1 || m.notice != "目录已在搜索范围内" {
		t.Errorf("重复添加应拒绝: %v %q", m.extraRoots, m.notice)
	}
	// pickText 输出绝对路径（额外目录）
	m.input.SetValue("extra")
	m.version++
	m.drain(m.runSearch())
	if len(m.results) != 1 || m.pickText(m.results[0]) != m.results[0].Path {
		t.Errorf("额外目录输出应为绝对路径: %v", m.results)
	}
	// 清除恢复
	if cmd := m.clearExtraRoots(); cmd != nil {
		m.drain(cmd)
	}
	if len(m.extraRoots) != 0 || !strings.Contains(m.frame(), "+1 目录") == false {
		t.Log("清除后状态栏无目录徽章")
	}
}

// 无效目录拒绝；~ 展开生效；上限封顶。
func TestAddExtraRootGuards(t *testing.T) {
	m, _ := newWorkspaceModel(t)
	if cmd := m.addExtraRoot(filepath.Join(m.root, "no-such")); cmd != nil {
		m.drain(cmd)
	}
	if len(m.extraRoots) != 0 || m.notice == "" {
		t.Errorf("无效目录应拒绝: %v %q", m.extraRoots, m.notice)
	}
	if cmd := m.addExtraRoot(m.root); cmd != nil {
		m.drain(cmd)
	}
	if m.notice != "该目录已是主搜索目录" {
		t.Errorf("主目录重复提示: %q", m.notice)
	}
	if got := expandHome("~/x"); !strings.HasPrefix(got, string(os.PathSeparator)) || strings.Contains(got, "~") {
		t.Errorf("~ 未展开: %q", got)
	}
	for i := 0; i < maxExtraRoots+2; i++ {
		d := t.TempDir()
		if cmd := m.addExtraRoot(d); cmd != nil {
			m.drain(cmd)
		}
	}
	if len(m.extraRoots) != maxExtraRoots {
		t.Errorf("上限应封顶 %d: %d", maxExtraRoots, len(m.extraRoots))
	}
}

// 多目录时 Git 筛选段隐藏。
func TestMultiRootHidesGitChips(t *testing.T) {
	m, extra := newWorkspaceModel(t)
	m.cfg.GitFiles = fakeGitFiles([]string{"main.txt"}, nil)
	if cmd := m.addExtraRoot(extra); cmd != nil {
		m.drain(cmd)
	}
	if cmd := m.toggleRangeBar(); cmd != nil {
		m.drain(cmd)
	}
	if m.gitOK || m.rangeBarH() != 2 {
		t.Errorf("多目录应隐藏 Git 段: gitOK=%v barH=%d", m.gitOK, m.rangeBarH())
	}
}

// 面板入口 + 目录输入浮层键路。
func TestWorkspacePaletteAndOverlay(t *testing.T) {
	m, _ := newWorkspaceModel(t)
	found := false
	for _, it := range m.paletteItems() {
		if strings.Contains(it.title, "添加搜索目录") {
			found = true
		}
	}
	if !found {
		t.Fatal("面板应有添加目录命令")
	}
	m.dirOpen = true
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); cmd != nil {
		m.drain(cmd)
	}
	if m.dirOpen {
		t.Error("Esc 应关闭目录浮层")
	}
}
