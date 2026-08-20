// Package logs 提供运行时日志源（Docker 容器等）的快照抓取，
// 抓取结果落盘为普通文件，复用 scx-rg 既有的搜索与预览能力。
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

// Runner 执行 docker 子命令并返回 stdout；可注入 fake 以便测试。
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// DefaultRunner 真实执行 docker 命令。
func DefaultRunner(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", args...).Output()
}

// DockerAvailable 检测系统是否安装 docker。
func DockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// SnapshotDocker 抓取容器最近 tail 行日志（含时间戳）写入临时文件，
// 返回文件路径；调用方负责在用完后删除。
func SnapshotDocker(ctx context.Context, run Runner, container string, tail int) (string, error) {
	if run == nil {
		run = DefaultRunner
	}
	out, err := run(ctx, "logs", "--timestamps", "--tail", strconv.Itoa(tail), container)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("docker logs 失败: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("docker logs 失败: %w", err)
	}
	f, err := os.CreateTemp("", "scx-rg-docker-*.log")
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
