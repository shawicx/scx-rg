package search

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
)

// GitLogProvider git log -G<pattern> 流式搜索：找出引入/删除了某段代码的
// 提交。每个 commit 一条结果：Path=短hash、Text="短hash 日期 subject"、
// Detail=完整 hash（详情与定位用）。结构与 RipgrepProvider 一致：流式
// 发送、ctx 可取消、错误以 Result.Err 送达。
type GitLogProvider struct{}

func (GitLogProvider) Name() string { return "gitlog" }

// ParseGitLogLine 解析 --pretty=%h|%ad|%s|%H 一行；不合法返回 ok=false。
func ParseGitLogLine(line string) (Result, bool) {
	line = strings.TrimRight(line, "\r")
	parts := strings.SplitN(line, "|", 4)
	if len(parts) != 4 || len(parts[0]) < 6 || len(parts[3]) < 6 {
		return Result{}, false
	}
	return Result{
		Path:   parts[0],
		Text:   parts[0] + " " + parts[1] + " " + parts[2],
		Detail: parts[3],
	}, true
}

func (GitLogProvider) SearchStream(ctx context.Context, root, query string) (<-chan Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("Git 历史需要输入关键词（git log -G 模式）")
	}
	// -M 启用重命名检测；--date=short 紧凑日期；subject 单行
	args := []string{"log", "-G", query, "-M", "--date=short",
		"--pretty=format:%h|%ad|%s|%H"}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	ch := make(chan Result, 128)
	go func() {
		defer close(ch)
		defer func() { _ = cmd.Wait() }()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		count := 0
		for sc.Scan() {
			res, ok := ParseGitLogLine(sc.Text())
			if !ok {
				continue
			}
			count++
			select {
			case ch <- res:
			case <-ctx.Done():
				return
			}
		}
	}()
	// 消费端取消时确保进程退出：cancel 由调用方在流结束后触发
	var once sync.Once
	ctxDone := ctx.Done()
	if ctxDone != nil {
		go func() {
			<-ctxDone
			once.Do(func() {
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			})
		}()
	}
	return ch, nil
}
