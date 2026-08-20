package logs

import (
	"context"
	"strings"
	"testing"
)

func TestListSourcesDocker(t *testing.T) {
	out := strings.Join([]string{
		`{"Names":"web","Image":"nginx:stable","State":"running","Status":"Up 2 days"}`,
		`{"Names":"api","Image":"repo/api:2.4","State":"exited","Status":"Exited (0) 1 hour ago"}`,
	}, "\n")
	var gotArgs []string
	fake := func(ctx context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(out), nil
	}
	srcs, err := ListSources(context.Background(), fake, "docker")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ps", "-a", "--format", "{{json .}}"}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("docker ps 参数 = %v, 期望 %v", gotArgs, want)
	}
	if len(srcs) != 2 {
		t.Fatalf("应解析出 2 个容器, 得到 %d", len(srcs))
	}
	web := srcs[0]
	if web.Target.Kind != "docker" || web.Target.Name != "web" {
		t.Fatalf("web Target 错误: %+v", web.Target)
	}
	if web.Detail != "nginx:stable" || web.Status != "Up 2 days" {
		t.Fatalf("web 展示信息错误: %+v", web)
	}
	if srcs[1].Target.Name != "api" {
		t.Fatalf("第二个容器应为 api: %+v", srcs[1])
	}
}

func TestListSourcesKubectl(t *testing.T) {
	out := `{"items":[
		{"metadata":{"name":"api-7d9xk","namespace":"prod"},
		 "status":{"phase":"Running","containerStatuses":[{"ready":true},{"ready":false}]}},
		{"metadata":{"name":"worker-1","namespace":"default"},
		 "status":{"phase":"Pending","containerStatuses":[]}}
	]}`
	var gotArgs []string
	fake := func(ctx context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(out), nil
	}
	srcs, err := ListSources(context.Background(), fake, "kubectl")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"get", "pods", "-o", "json"}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("kubectl 参数 = %v, 期望 %v", gotArgs, want)
	}
	if len(srcs) != 2 {
		t.Fatalf("应解析出 2 个 Pod, 得到 %d", len(srcs))
	}
	api := srcs[0]
	if api.Target.Kind != "kubectl" || api.Target.Name != "api-7d9xk" || api.Target.Namespace != "prod" {
		t.Fatalf("api Target 错误: %+v", api.Target)
	}
	if api.Detail != "prod" {
		t.Fatalf("api namespace 展示错误: %q", api.Detail)
	}
	if api.Status != "1/2 Running" {
		t.Fatalf("api 状态应为 1/2 Running, 得到 %q", api.Status)
	}
	if srcs[1].Status != "Pending" {
		t.Fatalf("worker 状态应为 Pending, 得到 %q", srcs[1].Status)
	}
}

func TestListSourcesError(t *testing.T) {
	fake := func(ctx context.Context, args ...string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	}
	if _, err := ListSources(context.Background(), fake, "docker"); err == nil {
		t.Fatal("命令失败应返回错误")
	}
}

func TestListSourcesEmpty(t *testing.T) {
	fake := func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("\n\n"), nil // 空行不应报错
	}
	srcs, err := ListSources(context.Background(), fake, "docker")
	if err != nil || len(srcs) != 0 {
		t.Fatalf("空输出应为空列表无错误, 得到 %v %d", err, len(srcs))
	}
}
