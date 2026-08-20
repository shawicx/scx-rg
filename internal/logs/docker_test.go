package logs

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSnapshotDockerPassesRightArgsAndWritesFile(t *testing.T) {
	var gotArgs []string
	fake := func(ctx context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("2026-08-20T10:00:00Z line1\n2026-08-20T10:00:01Z line2\n"), nil
	}
	path, err := SnapshotDocker(context.Background(), fake, "web", 100000)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	want := []string{"logs", "--timestamps", "--tail", "100000", "web"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("docker 参数 = %v, 期望 %v", gotArgs, want)
	}
	if !strings.HasSuffix(path, ".log") {
		t.Fatalf("快照文件应以 .log 结尾: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "2026-08-20T10:00:00Z line1\n2026-08-20T10:00:01Z line2\n" {
		t.Fatalf("快照内容应与 docker logs 输出一致，得到 %q", data)
	}
}

func TestSnapshotDockerPropagatesError(t *testing.T) {
	fake := func(ctx context.Context, args ...string) ([]byte, error) {
		return nil, os.ErrPermission
	}
	_, err := SnapshotDocker(context.Background(), fake, "web", 1000)
	if err == nil {
		t.Fatal("docker 命令失败应返回错误")
	}
	if !strings.Contains(err.Error(), "docker logs") {
		t.Fatalf("错误信息应说明来源: %v", err)
	}
}
