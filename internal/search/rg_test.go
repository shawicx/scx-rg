package search

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// drain 在超时内读完 channel，返回收到的结果数与是否正常关闭。
func drain(ch <-chan Result, timeout time.Duration) (int, bool) {
	n := 0
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return n, true
			}
			n++
		case <-time.After(timeout):
			return n, false
		}
	}
}

func TestSearchStreamNoMatchIsNotError(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := writeTree(t, map[string]string{"a.txt": "hello\n"}, false)
	ch, err := (RipgrepProvider{}).SearchStream(context.Background(), dir, "zzz_not_exist")
	if err != nil {
		t.Fatal(err)
	}
	n, closed := drain(ch, 5*time.Second)
	if !closed {
		t.Fatal("channel 应正常关闭")
	}
	if n != 0 {
		t.Fatalf("无匹配应静默结束（rg 退出码 1 是正常语义），不应传回错误结果，收到 %d 条", n)
	}
}

func TestSearchStreamBadRegexSurfacesError(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := writeTree(t, map[string]string{"a.txt": "hello\n"}, false)
	ch, err := (RipgrepProvider{}).SearchStream(context.Background(), dir, "(unclosed")
	if err != nil {
		t.Fatal(err)
	}
	var gotErr error
	n := 0
	for r := range ch {
		n++
		if r.Err != nil {
			gotErr = r.Err
		}
	}
	if gotErr == nil {
		t.Fatalf("非法正则应通过结果流传回错误（当前收到 %d 条结果）", n)
	}
	if gotErr.Error() == "" {
		t.Fatal("错误信息不应为空")
	}
}

func TestSearchStreamFindsMatches(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := writeTree(t, map[string]string{
		"a.txt": "hello\nneedle here\n",
		"b.txt": "nothing",
	}, false)
	ch, err := (RipgrepProvider{}).SearchStream(context.Background(), dir, "needle")
	if err != nil {
		t.Fatal(err)
	}
	n, closed := drain(ch, 5*time.Second)
	if !closed {
		t.Fatal("channel 应在搜索结束后关闭")
	}
	if n != 1 {
		t.Fatalf("匹配数 = %d, 期望 1", n)
	}
}

func TestSearchStreamEmptyQuery(t *testing.T) {
	ch, err := (RipgrepProvider{}).SearchStream(context.Background(), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if n, closed := drain(ch, time.Second); !closed || n != 0 {
		t.Fatalf("空查询应立即关闭空 channel，得到 n=%d closed=%v", n, closed)
	}
}

func TestSearchStreamEmptyQueryAllowedMatchesAll(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := writeTree(t, map[string]string{
		"a.log": "first line\nsecond line\n",
		"b.log": "third line\n",
	}, false)
	ch, err := (RipgrepProvider{AllowEmptyQuery: true}).SearchStream(context.Background(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	n, closed := drain(ch, 5*time.Second)
	if !closed {
		t.Fatal("channel 应在搜索结束后关闭")
	}
	if n != 3 {
		t.Fatalf("空查询放行应匹配每一行（rg 空模式），得到 n=%d", n)
	}
}

func TestSearchStreamCancelClosesPromptly(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	big := ""
	for i := 0; i < 3000; i++ {
		big += fmt.Sprintf("line %d has needle\n", i)
	}
	dir := writeTree(t, map[string]string{"big.txt": big}, false)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := (RipgrepProvider{}).SearchStream(ctx, dir, "needle")
	if err != nil {
		t.Fatal(err)
	}
	// 读几条确认流已启动，然后取消
	for i := 0; i < 3; i++ {
		select {
		case _, ok := <-ch:
			if !ok {
				t.Fatal("刚启动不应已关闭")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("等待首批结果超时")
		}
	}
	start := time.Now()
	cancel()
	// buffer 满时生产者会阻塞在发送上，必须靠 ctx.Done 解除并关闭 channel
	if _, closed := drain(ch, 3*time.Second); !closed {
		t.Fatal("取消后 channel 应 promptly 关闭（生产者不能泄漏）")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("关闭耗时 %v，取消应立即生效", elapsed)
	}
}

func TestSearchStreamStreamsAllMatches(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	files := map[string]string{}
	for f := 0; f < 30; f++ { // 30 文件 × 每文件 20 处匹配 = 600 条候选
		content := ""
		for i := 0; i < 20; i++ {
			content += "needle line\n"
		}
		files[fmt.Sprintf("f%02d.txt", f)] = content
	}
	dir := writeTree(t, files, false)
	ch, err := (RipgrepProvider{}).SearchStream(context.Background(), dir, "needle")
	if err != nil {
		t.Fatal(err)
	}
	n, closed := drain(ch, 10*time.Second)
	if !closed {
		t.Fatal("流应正常收尾关闭")
	}
	// 生产者不再截断：截多少由消费者决定（日志模式要保留最新窗口）
	if n != 600 {
		t.Fatalf("流式结果数 = %d, 期望 600（不截断）", n)
	}
}

func TestSearchStreamLiteralFixedStrings(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := writeTree(t, map[string]string{"a.log": "boom log.error( here\nplain line\n"}, false)

	// 正则模式：未闭合括号 → 报错，且错误信息不重复 rg: 前缀
	ch, err := (RipgrepProvider{}).SearchStream(context.Background(), dir, "log.error(")
	if err != nil {
		t.Fatal(err)
	}
	var gotErr error
	for r := range ch {
		if r.Err != nil {
			gotErr = r.Err
		}
	}
	if gotErr == nil {
		t.Fatal("正则模式下未闭合括号应报错")
	}
	if msg := gotErr.Error(); !strings.Contains(msg, "regex parse error") || strings.Contains(msg, "rg: rg:") {
		t.Fatalf("错误信息应含 regex parse error 且无重复前缀: %q", msg)
	}

	// 字面量模式：作为固定字符串精确命中该行
	ch2, err := (RipgrepProvider{Literal: true}).SearchStream(context.Background(), dir, "log.error(")
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for r := range ch2 {
		if r.Err != nil {
			t.Fatalf("字面量模式不应报错: %v", r.Err)
		}
		texts = append(texts, r.Text)
	}
	if len(texts) != 1 || texts[0] != "boom log.error( here" {
		t.Fatalf("字面量模式应只命中该行，得到 %v", texts)
	}
}

func TestRgErrMessage(t *testing.T) {
	stderr := "rg: regex parse error:\n    log.error(\n             ^\nerror: unclosed group\n"
	if got, want := rgErrMessage(stderr), "regex parse error: unclosed group"; got != want {
		t.Fatalf("rgErrMessage = %q, 期望 %q", got, want)
	}
	if got := rgErrMessage(""); got != "" {
		t.Fatalf("空 stderr 应返回空: %q", got)
	}
}
