# AI Code Agent

以下规则适用于本项目，AI 在修改代码前必须严格遵守。

---

## 1. 核心原则

0. **先了解项目再动手**
   - 执行任务或修改代码之前，如果需要了解项目，必须先阅读 `.wiki/` 中的项目概述和文档结构
   - 掌握项目的整体架构和设计原则后再开始工作，避免基于猜测的修改

1. **保持现有功能完整性**
   - 除非用户明确要求，不得修改现有功能行为、配置、接口、环境变量结构、目录结构、脚手架流程
   - 保持项目原有的构建流程和运行方式

2. **最小化修改**
   - 只做必要的修改，避免影响不相关的功能模块
   - 新增逻辑必须最小化修改范围，避免连锁兼容问题

3. **代码风格统一**
   - 遵循项目现有的代码风格和命名规范
   - 文件名统一使用 kebab-case 或遵循项目现有约定
   - 保持文件和目录结构的一致性

---

## 2. 依赖管理

1. **禁止降级依赖版本**，只有用户明确允许或必须降级以修复冲突时才可降版本且经过用户允许
2. **不得移除现有依赖**，除非用户明确要求或明确冗余且经过用户允许
3. **新增依赖必须遵循最新兼容版本策略（semver ^）**，并考虑兼容性
4. **修改依赖配置前必须先确认项目类型与构建体系**

---

## 3. 输出要求

1. 提供清晰的代码实现，优先输出代码与必要的命令步骤
2. 必要时说明修改原因，避免冗余解释性文本
3. 不得擅自生成总结文档、README 或说明文档（除非用户明确要求）

---

## 4. 禁止事项

以下行为全部禁止：

- 自动执行 `git commit`、`git push` 等提交操作（代码修改完成后由用户审查并手动提交）
- 擅自重构项目一级目录结构（如 src、public、dist、apps 等）
- 改变 lint / build 行为导致结果不同
- 将项目改为其他框架或运行环境
- 删除现有功能
- 擅自改变基础配置文件（如 tsconfig / vite / webpack / Cargo.toml / pom.xml）
- 插入当前环境无法使用的 API（例如在浏览器项目引入 fs/path 等 node-only API）
- **Superpowers spec/plan 文件错放**：spec 与 plan 文件只能存放在 `docs/superpowers/specs/` 和 `docs/superpowers/plans/` 下；禁止从 `.gitignore` 中删除 `docs/superpowers` 条目；禁止将 spec/plan 文件放到任何其他目录

---

## 5. 注释规范

**示例（TypeScript）：**

```typescript
/**
 * @description 从 Apifox 平台获取 OpenAPI 数据
 * @param config API 配置对象
 * @returns Promise<ApiData> API 数据
 *
 * @example const data = await fetchApifoxData({ source: '...', token: '...' });
 *
 */
```

**强制要求：**

1. 每个文件必须有 `@description` 文件头注释（中文）
2. 每个函数必须有 `@description` 注释
3. 有参数的函数必须有 `@param` 注释
4. 有返回值的函数必须有 `@returns` 注释
5. 核心函数（命名策略、类型清理、生成器、解析器、转换器等）需要 `@example` 标签

**语言差异说明：**

- TypeScript / JavaScript：使用 JSDoc 风格（如上示例）
- Java：使用 Javadoc 风格（`/** ... */`），遵循 Java 既有规范
- Rust：使用 `///` 文档注释，遵循 rustdoc 规范（`# Arguments` / `# Returns` / `# Examples` 段）
- 各语言按各自规范实现上述 5 条要求的内容，不强制使用完全相同的标签名

---


---

## Go CLI 项目规则

### 构建工具与运行时

1. **必须使用 Go 官方工具链与 Go Modules**（`go.mod` / `go.sum`），不得切换 GOPATH 模式或引入其他包管理方案
2. **Go 语言版本必须与 `go.mod` 中 `go` 指令声明一致**，不得擅自升级或降级
3. CLI 框架遵循项目现状（cobra / urfave-cli / flag 等），不得擅自替换
4. 工具链版本（go、golangci-lint）遵循 `go.mod` / CI 配置 / 项目现状，不得擅自更改

### 项目结构约定

遵循 Go 标准 CLI 项目结构，不得擅自调整一级目录：

- `main.go` 或 `cmd/<name>/main.go` — 入口（保持精简，只做装配与启动）
- `cmd/` — 子命令实现（每个子命令一个文件或包）
- `internal/` — 私有实现代码（外部模块不可导入）
- `pkg/` — 可对外暴露的库代码（可选，遵循项目现状）
- `go.mod` / `go.sum` — 模块清单与校验文件（必须提交）

### 构建与测试命令

- `go build ./...` — 编译全部包
- `go run .` 或 `go run ./cmd/<name>` — 运行
- `go test ./...` — 运行所有测试
- `go test ./... -run <name>` — 运行指定测试
- `go fmt ./...` — 格式化
- `go vet ./...` — 静态检查
- `golangci-lint run` — lint 检查（如项目已配置）

### 依赖与配置规则

1. **禁止降级依赖版本**；`go get -u` 大范围升级需经用户批准
2. `go.mod` 修改采取增量合并策略，不得重写整个文件；`go.sum` 由工具维护，不得手工编辑
3. 公开标识符（导出函数 / 类型 / 常量）必须有 doc comment，且以标识符名称开头
4. 错误处理遵循项目现状（`errors.Is` / `errors.As` / `fmt.Errorf("%w")` 包装 / 自定义 error 类型），不得擅自替换方案
5. 新增依赖优先评估标准库（flag / os / bufio 等）能否满足，避免引入功能重叠的第三方库
6. 日志方案遵循项目现状（`log/slog` / logrus / zap 等），不得擅自替换

### 禁止事项

- 不得手改 `go.sum` 或删除校验条目来"绕过"依赖校验
- 不得使用 `panic` 处理可预期的运行时错误（仅限不可恢复场景）
- 不得忽略 `go vet` / lint 报警强行交付
- 不得绕过 `gofmt` 格式化规则引入个人代码风格
- 不得在 `internal/` 与 `pkg/` 之间随意搬移代码
- 不得在入口 `main.go` 中堆积业务逻辑
