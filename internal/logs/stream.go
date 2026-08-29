package logs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// streamLoop 逐行读 rc：每行剥掉行尾 CR/LF 后回调 onLine，同时以 LF 行
// 形式写入 path（O_TRUNC 起笔——实时会话的落盘从本次跟随开始，非历史拼接）。
// 读到 EOF 返回 nil；扫描/写盘失败返回错误。
func streamLoop(rc io.Reader, path string, onLine func(string)) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024) // 日志单行可达 MB 级
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if _, err := bw.WriteString(line + "\n"); err != nil {
			return err
		}
		if onLine != nil {
			onLine(line)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return bw.Flush()
}

// Stream 启动 `logs -f` 长驻进程（初始 tail 行与后续新行都流经 stdout）：
// 逐行 tee 到 path 并回调 onLine。进程退出后返回——容器停止是正常 EOF
// （返回 nil）；容器不存在等启动失败经 stderr 捕获返回错误；ctx 取消
// 返回 ctx 错误。取代旧 logs.Follow（落盘临时文件、无回调）。
func Stream(ctx context.Context, t Target, tail int, path string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, t.Bin(), followArgs(t, tail)...)
	var errBuf bytes.Buffer
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return err
	}
	rerr := streamLoop(stdout, path, onLine)
	werr := cmd.Wait()
	if rerr != nil {
		return rerr
	}
	if werr != nil && ctx.Err() == nil {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return fmt.Errorf("%s %s: %s", t.Bin(), t.Name, msg)
		}
		return fmt.Errorf("%s %s 异常退出: %w", t.Bin(), t.Name, werr)
	}
	return nil
}

// LivePath 实时 tee 落盘的稳定路径：base/<kind>/[<ns>/]<name>.log。
// kubectl 按 namespace 分目录（未指定用 default）。容器/Pod/namespace 名
// 字符集为 [a-zA-Z0-9][a-zA-Z0-9_.-]*，可直接作路径段。路径稳定可预测是
// 「默认 scx-rg 命令搜日志」成立的前提（含 --follow 边跟边搜）。
func LivePath(base string, t Target) string {
	if t.Kind == "kubectl" {
		ns := t.Namespace
		if ns == "" {
			ns = "default"
		}
		return filepath.Join(base, "kubectl", ns, t.Name+".log")
	}
	return filepath.Join(base, "docker", t.Name+".log")
}
