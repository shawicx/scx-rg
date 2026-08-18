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

func (p RipgrepProvider) Search(ctx context.Context, root, query string) ([]Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rg", "--json", "--smart-case", "--max-count", "20", "--", query, ".")
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var results []Result
	stopped := false
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
		results = append(results, Result{
			Path: path,
			Line: ev.Data.LineNumber,
			Text: strings.TrimSpace(ev.Data.Lines.Text),
		})
		if len(results) >= MaxResults {
			stopped = true
			break
		}
	}
	if stopped {
		cancel()
	}
	_ = cmd.Wait()
	return results, nil
}
