package logs

import (
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
