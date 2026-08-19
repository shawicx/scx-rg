package search

import (
	"context"
	"fmt"
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

func TestSearchStreamCapsAtMaxResults(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	files := map[string]string{}
	for f := 0; f < 30; f++ { // 30 文件 × 每文件 20 处匹配（--max-count 20）= 600 条候选
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
		t.Fatal("达到上限后应正常收尾关闭")
	}
	if n != MaxResults {
		t.Fatalf("流式结果数 = %d, 期望封顶 %d", n, MaxResults)
	}
}
