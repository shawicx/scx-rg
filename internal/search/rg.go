package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

// RipgrepProvider 借助 ripgrep 做内容搜索，流式解析 rg --json 输出。
// Literal 为 true 时用固定字符串匹配（-F），适合搜索含正则元字符的文本
// （如 log.error(）；默认按正则解析查询。
type RipgrepProvider struct {
	Literal bool
}

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
	args := []string{"--json", "--smart-case"}
	if p.Literal {
		args = append(args, "--fixed-strings")
	}
	args = append(args, "--", query, ".")
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = root
	// 捕获 stderr：rg 对权限错误/非法正则的报错不能漏进 TUI，失败时经结果流传回
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
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
		}
		waitErr := cmd.Wait()
		// rg 退出码语义：0=有匹配，1=无匹配（正常），2+=错误。
		// 仅在「非零退出且不是无匹配」且一条结果都没发出时，把 stderr 摘要传回调用方。
		if waitErr != nil && count == 0 && ctx.Err() == nil {
			var ee *exec.ExitError
			if !errors.As(waitErr, &ee) || ee.ExitCode() != 1 {
				msg := rgErrMessage(stderr.String())
				if msg == "" {
					msg = waitErr.Error()
				}
				select {
				case ch <- Result{Err: errors.New(msg)}:
				case <-ctx.Done():
				}
			}
		}
	}()
	return ch, nil
}

// rgErrMessage 从 rg 的 stderr 里提炼一行可读错误：首行去掉 rg 自带的
// "rg: " 前缀（避免出现 rg: rg: 重复），再拼上末尾 "error: xxx" 的细节行。
func rgErrMessage(stderr string) string {
	lines := strings.Split(stderr, "\n")
	msg := ""
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			msg = strings.TrimPrefix(l, "rg: ")
			break
		}
	}
	for _, l := range lines {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "error:") {
			msg += " " + strings.TrimSpace(strings.TrimPrefix(l, "error:"))
		}
	}
	return strings.TrimSpace(msg)
}
