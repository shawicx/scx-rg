# CI 与发版

## CI（.github/workflows/ci.yml）

触发：push 到 main、PR。矩阵 ubuntu-latest + macos-latest，步骤：

1. **Format check**——`gofmt -l .` 非空即列文件并 fail（M5 补）
2. **Vet**——`go vet ./...`
3. **Test**——`go test ./...`（含 golden frame 对比）
4. **Smoke render**（仅 ubuntu）——`go run . --once -q rg > /dev/null`（--once 单帧渲染即冒烟）
5. **GoReleaser config check**（仅 ubuntu）——`goreleaser check`
6. **git-cliff config check**（仅 ubuntu）——`--unreleased` 试跑变更记录生成

## 发版（.github/workflows/release.yml）

版本号以 git tag 为唯一来源，**发版不改任何代码**：

```bash
git tag v0.1.0 && git push origin v0.1.0
```

流水：checkout（fetch-depth 0）→ `go test ./...` → git-cliff `--latest` 生成 RELEASE_NOTES.md → goreleaser `release --clean --release-notes=...` 创建 GitHub Release。

- **交叉编译**：macOS/Linux × amd64/arm64 压缩包 + sha256 校验和（goreleaser 配置）
- **Release 正文**：git-cliff 按 conventional commit 前缀生成中文分组变更记录（`🚀 新功能` / `🐛 Bug 修复` 等，配置 cliff.toml）
- **版本注入**：version.go 的变量由 goreleaser ldflags 注入；本地 `go build` 显示 `dev`

## 提交规范

commitlint 风格 `<type>: <中文描述>`（feat/fix/docs/style/refactor/perf/test/build/ci/chore/revert）——git-cliff 按前缀分组，不规范前缀会掉进未分组。

## 仓库级忽略

`dist/`（goreleaser 产物）、编译二进制（scx-rg*）等见 .gitignore。

Related: [testing](testing.md) · [README 发版章节](../../README.md)
