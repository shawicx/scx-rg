package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Source 一个可选的日志目标（容器 / Pod），供选择器展示。
type Source struct {
	Target Target
	Detail string // 镜像 / namespace 等副标题
	Status string // Up 2 days / 1/2 Running
}

// ListSources 列出指定类型的日志目标。docker 用 `ps -a --format json`（JSON 行），
// kubectl 用 `get pods -o json`；run 可注入 fake 以便测试。
func ListSources(ctx context.Context, run Runner, kind string) ([]Source, error) {
	t := Target{Kind: kind}
	if run == nil {
		run = func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, t.Bin(), args...).Output()
		}
	}
	var (
		out []byte
		err error
	)
	if kind == "kubectl" {
		out, err = run(ctx, "get", "pods", "-o", "json")
	} else {
		out, err = run(ctx, "ps", "-a", "--format", "{{json .}}")
	}
	if err != nil {
		return nil, fmt.Errorf("列出%s失败: %w", t.Bin(), err)
	}
	if kind == "kubectl" {
		return parsePods(out)
	}
	return parseContainers(out)
}

type psRow struct {
	Names string `json:"Names"`
	Image string `json:"Image"`
	State string `json:"State"`
	Size  string `json:"Status"`
}

func parseContainers(out []byte) ([]Source, error) {
	var srcs []Source
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row psRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue // 跳过无法解析的行（docker 版本差异等）
		}
		if row.Names == "" {
			continue
		}
		srcs = append(srcs, Source{
			Target: Target{Kind: "docker", Name: row.Names},
			Detail: row.Image,
			Status: row.Size,
		})
	}
	return srcs, nil
}

type podsJSON struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Status struct {
			Phase             string `json:"phase"`
			ContainerStatuses []struct {
				Ready bool `json:"ready"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

func parsePods(out []byte) ([]Source, error) {
	var pods podsJSON
	if err := json.Unmarshal(out, &pods); err != nil {
		return nil, fmt.Errorf("解析 kubectl get pods 输出失败: %w", err)
	}
	var srcs []Source
	for _, item := range pods.Items {
		if item.Metadata.Name == "" {
			continue
		}
		status := item.Status.Phase
		if n := len(item.Status.ContainerStatuses); n > 0 {
			ready := 0
			for _, c := range item.Status.ContainerStatuses {
				if c.Ready {
					ready++
				}
			}
			status = fmt.Sprintf("%d/%d %s", ready, n, item.Status.Phase)
		}
		srcs = append(srcs, Source{
			Target: Target{Kind: "kubectl", Name: item.Metadata.Name, Namespace: item.Metadata.Namespace},
			Detail: item.Metadata.Namespace,
			Status: status,
		})
	}
	return srcs, nil
}
