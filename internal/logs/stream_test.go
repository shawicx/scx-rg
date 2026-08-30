package logs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamLoopLinesAndTee(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.log")
	var got []string
	err := streamLoop(strings.NewReader("l1\r\nl2\nl3\n"), path, func(l string) { got = append(got, l) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "l1|l2|l3" {
		t.Fatalf("回调行序错误: %v", got)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "l1\nl2\nl3\n" {
		t.Fatalf("tee 内容应为 CRLF 归一后的三行: %q", b)
	}
}

func TestStreamLoopTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.log")
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := streamLoop(strings.NewReader("new\n"), path, nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "new\n" {
		t.Fatalf("起笔应 truncate 旧会话内容: %q", b)
	}
}

// TestStreamLoopFlushesPerLine 落盘的逐行中途可见性：onLine 回调发生时该
// 行必须已在文件里——tee 文件的增长是 `scx-rg --follow <落盘文件>` 的
// 检测信号，bufio 缓冲到流结束才 Flush 会让活跃会话恒缺最新 <4KB。
// 流结束前无法断言「中途」，故在回调内就地读文件验证；两行总长远小于
// 4KB，确保不靠缓冲写满落盘蒙混过关。
func TestStreamLoopFlushesPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.log")
	var calls int
	err := streamLoop(strings.NewReader("short-one\nshort-two\n"), path, func(l string) {
		calls++
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Errorf("第 %d 行回调时文件应已可读: %v", calls, rerr)
			return
		}
		if !strings.Contains(string(b), l+"\n") {
			t.Errorf("第 %d 行回调时文件应已含该行（逐行落盘）: 文件=%q 当前行=%q", calls, b, l)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("应回调 2 行, got %d", calls)
	}
}

// TestStreamLoopTeeErrorIsolated 落盘失败只降级不杀流（spec §9：落盘失败
// 不影响面板渲染）：tee 目标被同名目录占位使 OpenFile 失败时，错误以 ⚠
// 行进入回调流（面板立即可见），输入行照常回调，streamLoop 返回 nil——
// 若上抛错误会连内存缓冲一起杀掉，面板反而整段丢日志。
func TestStreamLoopTeeErrorIsolated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.log")
	if err := os.Mkdir(path, 0o755); err != nil { // 同名目录：OpenFile 必失败
		t.Fatal(err)
	}
	var got []string
	err := streamLoop(strings.NewReader("l1\nl2\n"), path, func(l string) { got = append(got, l) })
	if err != nil {
		t.Fatalf("落盘失败应隔离为 ⚠ 行而非返回错误: %v", err)
	}
	joined := strings.Join(got, "|")
	if !strings.Contains(joined, "l1") || !strings.Contains(joined, "l2") {
		t.Fatalf("输入行应照常回调: %v", got)
	}
	if !strings.Contains(joined, "⚠ 落盘写入失败，已停止落盘") {
		t.Fatalf("落盘失败应以 ⚠ 行入回调流: %v", got)
	}
}

func TestLivePath(t *testing.T) {
	if p := LivePath("/cache", Target{Kind: "docker", Name: "web"}); p != "/cache/docker/web.log" {
		t.Fatalf("docker 路径: %s", p)
	}
	if p := LivePath("/cache", Target{Kind: "kubectl", Name: "pod-1"}); p != "/cache/kubectl/default/pod-1.log" {
		t.Fatalf("kubectl 默认 ns: %s", p)
	}
	if p := LivePath("/cache", Target{Kind: "kubectl", Name: "pod-1", Namespace: "prod"}); p != "/cache/kubectl/prod/pod-1.log" {
		t.Fatalf("kubectl 指定 ns: %s", p)
	}
}

func TestStreamCommandMergesStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.log")
	var got []string
	err := streamCommand(context.Background(), "sh", []string{"-c", "echo out-line; echo err-line >&2"}, path, func(l string) { got = append(got, l) })
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "|")
	if !strings.Contains(joined, "out-line") || !strings.Contains(joined, "err-line") {
		t.Fatalf("stdout/stderr 应合流进回调: %v", got)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "out-line") || !strings.Contains(string(b), "err-line") {
		t.Fatalf("tee 文件应同时含两流: %q", b)
	}
}

func TestStreamCommandErrorExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.log")
	var got []string
	err := streamCommand(context.Background(), "sh", []string{"-c", "echo boom >&2; exit 3"}, path, func(l string) { got = append(got, l) })
	if err == nil {
		t.Fatal("非零退出且非 ctx 取消应返回错误")
	}
	if !strings.Contains(strings.Join(got, "|"), "boom") {
		t.Fatalf("stderr 错误文本应作为日志行入流: %v", got)
	}
}
