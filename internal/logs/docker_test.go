package logs

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSnapshotDockerArgsAndFile(t *testing.T) {
	var gotArgs []string
	fake := func(ctx context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("2026-08-21T10:00:00Z line1\n"), nil
	}
	target := Target{Kind: "docker", Name: "web"}
	path, err := Snapshot(context.Background(), fake, target, 100000)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	want := []string{"logs", "--timestamps", "--tail", "100000", "web"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("docker 参数 = %v, 期望 %v", gotArgs, want)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "2026-08-21T10:00:00Z") {
		t.Fatalf("快照内容错误: %q", data)
	}
}

func TestSnapshotKubectlArgs(t *testing.T) {
	cases := []struct {
		name   string
		target Target
		want   []string
	}{
		{
			name:   "仅 pod",
			target: Target{Kind: "kubectl", Name: "api-7d9x"},
			want:   []string{"logs", "api-7d9x", "--timestamps", "--tail", "50000"},
		},
		{
			name:   "namespace+container",
			target: Target{Kind: "kubectl", Name: "api-7d9x", Namespace: "prod", Container: "app"},
			want:   []string{"logs", "api-7d9x", "--timestamps", "--tail", "50000", "-n", "prod", "-c", "app"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotArgs []string
			fake := func(ctx context.Context, args ...string) ([]byte, error) { gotArgs = args; return nil, nil }
			path, err := Snapshot(context.Background(), fake, c.target, 50000)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(path)
			if !reflect.DeepEqual(gotArgs, c.want) {
				t.Fatalf("kubectl 参数 = %v, 期望 %v", gotArgs, c.want)
			}
		})
	}
}

func TestFollowArgs(t *testing.T) {
	docker := Target{Kind: "docker", Name: "web"}
	if got := followArgs(docker, 1000); !reflect.DeepEqual(got,
		[]string{"logs", "-f", "--timestamps", "--tail", "1000", "web"}) {
		t.Fatalf("docker follow 参数 = %v", got)
	}
	k8s := Target{Kind: "kubectl", Name: "api", Namespace: "prod"}
	if got := followArgs(k8s, 1000); !reflect.DeepEqual(got,
		[]string{"logs", "api", "-f", "--timestamps", "--tail", "1000", "-n", "prod"}) {
		t.Fatalf("kubectl follow 参数 = %v", got)
	}
}

func TestBinAndAvailable(t *testing.T) {
	if (Target{Kind: "docker"}).Bin() != "docker" {
		t.Fatal("docker")
	}
	if (Target{Kind: "kubectl"}).Bin() != "kubectl" {
		t.Fatal("kubectl")
	}
	// 可用性与本机是否安装一致（无论真假都应无 panic）
	_ = Target{Kind: "docker"}.Available()
	_ = Target{Kind: "kubectl"}.Available()
}

func TestSnapshotErrorPropagates(t *testing.T) {
	fake := func(ctx context.Context, args ...string) ([]byte, error) { return nil, os.ErrPermission }
	if _, err := Snapshot(context.Background(), fake, Target{Kind: "docker", Name: "w"}, 1000); err == nil {
		t.Fatal("应返回错误")
	}
}
