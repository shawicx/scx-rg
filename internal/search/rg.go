package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

// RipgrepProvider 借助 ripgrep 做内容搜索，流式解析 rg --json 输出。
type RipgrepProvider struct{}

// RgAvailable 检测系统是否安装了 rg。
func RgAvailable() bool {
	_, err := exec.LookPath("rg")
	return err == nil
}

func (RipgrepProvider) Name() string { return "content" }

type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path       rgText `json:"path"`
		Lines      rgText `json:"lines"`
		LineNumber int    `json:"line_number"`
	} `json:"data"`
}

type rgText struct {
	Text string `json:"text"`
}

// SearchStream 启动 rg 并通过 channel 流式返回匹配，全部发完（或达到上限、
// 被取消、出错）后关闭 channel。取消 ctx 会立即杀死 rg 进程并解除发送阻塞。
func (p RipgrepProvider) SearchStream(ctx context.Context, root, query string) (<-chan Result, error) {
	if strings.TrimSpace(query) == "" {
		ch := make(chan Result)
		close(ch)
		return ch, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, "rg", "--json", "--smart-case", "--max-count", "20", "--", query, ".")
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	ch := make(chan Result, 128)
	go func() {
		defer close(ch)
		defer cancel()
		count := 0
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			var ev rgEvent
			if json.Unmarshal(line, &ev) != nil || ev.Type != "match" {
				continue
			}
			path := strings.TrimPrefix(ev.Data.Path.Text, "./")
			if path == "" {
				continue
			}
			res := Result{
				Path: path,
				Line: ev.Data.LineNumber,
				Text: strings.TrimSpace(ev.Data.Lines.Text),
			}
			select {
			case ch <- res:
			case <-ctx.Done(): // 消费者停止读取时，靠取消解除发送阻塞
				_ = cmd.Wait()
				return
			}
			count++
			if count >= MaxResults {
				break
			}
		}
		_ = cmd.Wait()
	}()
	return ch, nil
}
