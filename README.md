![image.png](/assets/imgs/head_image_zh.png)
 简体中文 | [更新日志](./UpdateLog.md) | [帮助文档](https://goteams.dev/docs)


## 🎯 产品定位


Teams Desk Agent是新一代 AI 智能化研发管理平台，让 agent 成为你的团队成员。深度融合 AI 智能体，内置 agent 员工，让 Agent 成为你的团队成员。让每个团队都能轻松驾驭从需求到交付的全流程。

**
Teams Desk Agent客户端**是面向开发者与团队的开源桌面端，将官网的全部能力本地化运行：连接你的私有化部署服务器或官方云服务（`goteams.cn`），在本地客户端中管理我的任务、调试 Agent、构建知识库、编排工作流、对接 Git 与 API，获得更流畅的离线优先体验。
![image.png](/assets/imgs/ui_zh_1.png)

## ✨ 核心功能

### 🤖 AI Agent 协作

- **Agent 员工作为团队成员**：智能体出现在指派下拉菜单中，像分配任务给同事一样分配给 agent。
- **自主执行能力**：agent 主动领取任务、自主推进项目，完整的任务生命周期管理：入队、领取、启动、完成或失败。
- **主动报告与推送**：agent 遇到阻塞主动通知，通过实时推送获取进度更新。
- **统一活动时间线**：人类和 agent 的所有操作在统一时间线上可见，无缝协作。

### 📋 我的任务（客户端核心）

- **专属任务视图**：聚焦"我"的工作项，按状态、优先级、迭代筛选，一眼看清待办。
- **本地登录态**：支持官方服务器（`goteams.cn`）与私有化部署两种连接方式，登录信息本地安全存储。
- **需求详情速览与复制**：标题一键复制标题 / 链接 / 描述（支持"复制标题和链接""复制标题""复制链接""复制描述""复制标题链接描述"组合），便于对外同步。

### 🌐 更多功能

- **多种部署选项**：提供桌面客户端，支持连接官方云服务与私有化部署服务器，登录后可一键切换工作区。
- **Git 集成**：在客户端内对接代码仓库，关联工作项与提交记录。
- **项目配置**：集中管理项目字段、角色与流程，配置即生效。

## 🛸 用户界面

- 🌍 **免费体验网址**：[goteams.dev](https://goteams.dev/)
- 🖼️ **系统截图**：
  ![image.png](/assets/imgs/ui_zh_2.png)
  ![image.png](/assets/imgs/ui_zh_3.png)
  ![image.png](/assets/imgs/ui_zh_4.png)
  ![image.png](/assets/imgs/ui_zh_5.png)


## 🚀 快速开始

### 环境要求

- Go >= 1.26
- Node.js >= 18（开发模式 / 前端构建需要）
- npm >= 9
- go-task（构建任务管理器）

### 安装 go-task

```bash
go install github.com/go-task/task/v3/cmd/task@latest
```

安装完成后将 `$(go env GOPATH)/bin` 加入 `PATH`，执行 `task --version` 验证，`task` 可列出全部可用任务。

### 安装前端依赖

```bash
npm --prefix web install
```

### 打包桌面客户端

```bash
# 完整发布：先构建前端（web/dist），再交叉编译全部目标平台
task release

# 仅构建单个平台
task build:windows-amd64
```

- 启动后默认服务地址为：http://127.0.0.1:18900
- 支持平台：Windows / macOS / Linux × amd64 / arm64

### 客户端登录

启动客户端后进入登录页，选择 **官方** 或 **私有化部署**：
  - **官方**：服务器地址固定为 `goteams.cn`，无需修改。
  - **私有化部署**：填写你的服务器地址、账号与密码。

## 💻 技术栈

**后端**

- **语言**：Go 1.26
- **Web 框架**：Gin
- **本地数据库**：SQLite（modernc.org/sqlite，纯 Go 实现，支持 `CGO_ENABLED=0` 交叉编译）
- **实时通信**：gorilla/websocket（agent 进度实时推送）

**前端**

- **框架**：Vue 3 + TypeScript
- **构建工具**：Vite 6
- **UI 组件库**：Ant Design Vue 4

**构建与发布**

- **任务编排**：go-task（Taskfile.yml）
- **交叉编译**：tools/build（Windows / macOS / Linux × amd64 / arm64）

## 🏡 社区交流 & 联系我们

欢迎联系我们获取帮助，或者提供建议帮助我们改善 GoTeams 客户端。您可以通过以下方式联系我们：

- **邮箱**：发送邮件到 [contact@goteams.dev](mailto:contact@goteams.dev)
- **GitHub Issues**：[提交 Issue](https://github.com/your-org/goteams/issues)
- **官网**：[goteams.dev](https://goteams.dev/)

## 📖 更新日志

完整的更新日志请点击 👉️👉️ [CHANGELOG.md](./UpdateLog.md)

### 2026/08/05

1. 客户端「我的任务」登录页支持官方 / 私有化部署单选项切换，官方地址固定 `goteams.cn`
2. 需求详情标题复制改为悬停下拉，支持复制标题 / 链接 / 描述多种组合，字号 12px
3. 企业成员操作日志改为表格样式，新增按修改成员名称的搜索框
4. 企业成员新增历史日志图标，记录成员新增 / 删除、启用 / 禁用（含操作人、时间、修改成员）

### 2026/07/21

1. 项目详情增加批量修改迭代、批量删除（二次确认）
2. 项目角色权限新增"列表数据查看范围"Tab（全部成员 / 部分成员 / 仅自己）
3. 团队工作数据权限新增"仅自己"选项，普通成员默认仅自己
4. 官网主页新增二级标题"让 agent 成为你的团队成员"

### 2026/07/15

1. 需求 / 缺陷模块支持批量导出、批量修改状态 / 优先级 / 处理人
2. 工作项新增公司 / 产品线 / 应用 / 模块等维度的筛选字段
3. 迭代管理支持创建、编辑、删除和筛选
4. 新增自定义角色功能，支持功能权限配置

## 协议

本项目遵循 MIT 开源协议。