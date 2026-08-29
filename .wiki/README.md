# scx-rg Wiki

基于 ripgrep 的终端搜索浏览器与通用 fuzzy finder（Go + bubbletea 单二进制 TUI）。
本 Wiki 面向 AI 与开发者，内容基于仓库真实代码生成，路径与函数名可直接检索。

## 快速回答

| 问题 | 去哪 |
| --- | --- |
| 项目是什么、能做什么 | [01-overview/project-overview](01-overview/project-overview.md) |
| 模块怎么划分、调用链长什么样 | [01-overview/architecture](01-overview/architecture.md) |
| 主界面状态机与消息链 | [02-modules/tui](02-modules/tui.md) |
| 搜索/模糊匹配怎么工作 | [02-modules/search](02-modules/search.md) |
| 代码高亮与图片渲染 | [02-modules/preview](02-modules/preview.md) |
| docker/k8s 实时日志与日志搜索分离 | [02-modules/logs](02-modules/logs.md) |
| config.toml 支持什么 | [02-modules/config](02-modules/config.md) |
| 全部键位、多选、shell 集成 | [03-guides/interaction](03-guides/interaction.md) |
| 测试怎么跑、golden frame 是什么 | [03-guides/testing](03-guides/testing.md) |
| 怎么发版 | [03-guides/release](03-guides/release.md) |
| 为什么用 ASCII 边框、帧宽收窄 1 列 | [04-decisions/design-decisions](04-decisions/design-decisions.md) |

## 阅读路径

- **新人上手（10 分钟）**：project-overview → architecture → interaction（键位表）
- **改搜索/匹配逻辑**：search → tui（消息链部分）
- **改渲染/预览**：preview → design-decisions（终端兼容决策）
- **AI 代码导航**：architecture 有模块边界调用数与热点函数，配合 `internal/` 目录树使用

## 仓库内其他文档

- [README.md](../README.md)——用户视角的功能说明与用法
- [docs/PLAN.md](../docs/PLAN.md)——M1–M5 路线图与完成状态
- Wiki 不复制上述内容，冲突时以代码为准
