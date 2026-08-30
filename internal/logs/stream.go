package logs

import (
	"bufio"
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
// 读到 EOF 返回 nil；扫描失败返回错误。
// 落盘是逐行同步写、不经 bufio 缓冲：tee 文件的增长就是外部读端
// （`scx-rg --follow <落盘文件>`）检测新行的信号，缓冲到流结束才 Flush
// 会让活跃会话的落盘恒缺最新 <4KB，边跟边搜等于盲区；日志行频远低于
// 终端刷新量，逐行 write 的系统调用开销可忽略。
// 落盘失败被隔离而非上抛（spec §9：落盘失败不影响面板渲染）：首次失败
// （含 OpenFile 失败）以 ⚠ 行经 onLine 通知面板后一次性放弃落盘，扫描
// 与回调继续，流正常收束仍返回 nil——错误已作为日志行呈现，若再以返回
// 值上抛会杀掉整条流，面板反而丢日志。
func streamLoop(rc io.Reader, path string, onLine func(string)) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	// tee 为 nil 即「已停止落盘」：OpenFile 失败时 f 本就是 nil，首次
	// 写失败后置空——磁盘满/权限回收这类故障不会自愈，逐行重试只会
	// 反复报错，一次性降级为纯回调。
	tee := f
	if err != nil {
		if onLine != nil {
			onLine("⚠ 落盘写入失败，已停止落盘: " + err.Error())
		}
	} else {
		defer f.Close()
	}
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024) // 日志单行可达 MB 级
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if tee != nil {
			if _, werr := tee.WriteString(line + "\n"); werr != nil {
				tee = nil
				if onLine != nil {
					onLine("⚠ 落盘写入失败，已停止落盘: " + werr.Error())
				}
			}
		}
		if onLine != nil {
			onLine(line)
		}
	}
	return sc.Err()
}

// Stream 启动 `logs -f` 长驻进程（初始 tail 行与后续新行都流经输出流），
// 逐行 tee 到 path 并回调 onLine；stdout 与 stderr 合流处理（见
// streamCommand）。契约：err == nil ⇔ 干净结束——容器停止是正常 EOF，
// ctx 取消同样视为正常收束（返回 nil，与容器停止一致）；进程非零退出
// 且非取消时返回错误（启动错误文本已作为日志行入流）。
func Stream(ctx context.Context, t Target, tail int, path string, onLine func(string)) error {
	return streamCommand(ctx, t.Bin(), followArgs(t, tail), path, onLine)
}

// streamCommand 启动子进程并把 stdout 与 stderr 合流到同一管道后逐行
// 处理：docker CLI 对容器两个输出流是分流的（容器 stderr 走 CLI 自身
// stderr），只接 stdout 时纯 stderr 打日志的容器会整段丢失。合流的附带
// 收益：启动错误文本（如「No such container」）同样作为日志行入流，
// 无需缓冲 stderr——长驻会话里全量累积容器 stderr 会无界吃内存。
// 父侧写端在 Start 后立即关闭，保证子进程退出后读端能收到 EOF 而非
// 永久阻塞。进程非零退出且非 ctx 取消时返回错误。
func streamCommand(ctx context.Context, bin string, args []string, path string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return err
	}
	pw.Close() // 父侧写端立即关闭：子进程退出后读端才会 EOF
	rerr := streamLoop(pr, path, onLine)
	_ = pr.Close()
	werr := cmd.Wait()
	if rerr != nil {
		return rerr
	}
	if werr != nil && ctx.Err() == nil {
		return fmt.Errorf("%s 异常退出: %w", bin, werr)
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
