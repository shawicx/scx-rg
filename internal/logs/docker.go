// Package logs 提供运行时日志源（Docker 容器、Kubernetes Pod）的快照与
// 持续跟随抓取，结果落盘为普通文件，复用 scx-rg 既有的搜索与预览能力。
package logs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Runner 执行日志源命令并返回 stdout；可注入 fake 以便测试。
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// Target 统一日志目标：docker 容器或 kubectl pod。
type Target struct {
	Kind      string // "docker" | "kubectl"
	Name      string // 容器名 / Pod 名
	Namespace string // kubectl 专用
	Container string // kubectl 多容器 Pod 时指定
}

// Bin 返回日志源对应的命令行工具名。
func (t Target) Bin() string {
	if t.Kind == "kubectl" {
		return "kubectl"
	}
	return "docker"
}

// Available 检测本机是否安装对应命令行工具。
func (t Target) Available() bool {
	_, err := exec.LookPath(t.Bin())
	return err == nil
}

// DockerAvailable 兼容旧入口。
func DockerAvailable() bool {
	return Target{Kind: "docker"}.Available()
}

// snapshotArgs 拼装一次性快照参数。
func snapshotArgs(t Target, tail int) []string {
	if t.Kind == "kubectl" {
		args := []string{"logs", t.Name, "--timestamps", "--tail", strconv.Itoa(tail)}
		if t.Namespace != "" {
			args = append(args, "-n", t.Namespace)
		}
		if t.Container != "" {
			args = append(args, "-c", t.Container)
		}
		return args
	}
	return []string{"logs", "--timestamps", "--tail", strconv.Itoa(tail), t.Name}
}

// followArgs 拼装持续跟随参数（-f）。
func followArgs(t Target, tail int) []string {
	if t.Kind == "kubectl" {
		args := []string{"logs", t.Name}
		args = append(args, "-f")
		return appendKubectlOpts(append(args, "--timestamps", "--tail", strconv.Itoa(tail)), t)
	}
	return []string{"logs", "-f", "--timestamps", "--tail", strconv.Itoa(tail), t.Name}
}

func appendKubectlOpts(args []string, t Target) []string {
	if t.Namespace != "" {
		args = append(args, "-n", t.Namespace)
	}
	if t.Container != "" {
		args = append(args, "-c", t.Container)
	}
	return args
}

// Snapshot 抓取最近 tail 行日志（含时间戳）写入临时文件，返回文件路径；
// 调用方负责在用完后删除。
func Snapshot(ctx context.Context, run Runner, t Target, tail int) (string, error) {
	if run == nil {
		run = func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, t.Bin(), args...).Output()
		}
	}
	out, err := run(ctx, snapshotArgs(t, tail)...)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%s %s 失败: %s", t.Bin(), t.Name, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("%s %s 失败: %w", t.Bin(), t.Name, err)
	}
	f, err := os.CreateTemp("", "scx-rg-log-*.log")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(out); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Follow 启动 `logs -f` 长驻进程，把输出（含初始 tail 部分）持续 append 到 path；
// ctx 取消时进程自动被杀。适合 tail -f 式实时追新。
func Follow(ctx context.Context, t Target, tail int, path string) error {
	cmd := exec.CommandContext(ctx, t.Bin(), followArgs(t, tail)...)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd.Stdout = f
	cmd.Stderr = nil // 工具告警不混入日志
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return err
	}
	go func() {
		_ = cmd.Wait()
		_ = f.Close()
	}()
	return nil
}
