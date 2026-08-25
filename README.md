# GoTeams Client

GoTeams Client 是面向研发团队的桌面 Agent 客户端，使用 Electron 提供桌面窗口与原生能力，Vue 负责界面，Go sidecar 负责本地数据、工作流、CLI Agent、云端通信和 HTTP/WebSocket API。

## 技术栈

- 桌面：Electron 43、electron-builder
- 前端：Vue 3、TypeScript、Vite、Ant Design Vue、Pinia
- 后端：Go、Gin、SQLite、WebSocket
- 构建：go-task、npm、`tools/build`

详细的进程边界、目录职责、安全规范和扩展流程见 [总体架构与开发规范](./docs/架构设计/20260810_客户端架构与开发规范.md)。

## 环境要求

- Go 1.26+
- Node.js 24+
- npm 11+
- go-task 3.49+

安装依赖：

```bash
npm --prefix web install
npm --prefix desktop install
```

## 桌面开发

```bash
task dev       # 启动开发环境（Vite HMR + Electron + Go sidecar）
```

Electron 是应用父进程，会启动和监管 Go sidecar；Vite 仅在开发模式提供 HMR。关闭桌面客户端时，Go sidecar 会一并退出。

只调试单层：

```bash
npm --prefix web run dev:web   # 只启动 Vue/Vite
```

## 测试与检查

```bash
go test ./...              # Go 测试
go vet ./...               # Go 静态检查
npm --prefix web test      # Vue 类型检查与静态测试
npm --prefix desktop test  # Electron 类型检查与 Node 测试
```

## 打包

```bash
task pkg-win-dev1     # dev1 配置，Windows x64/arm64 安装包
task pkg-win-dev2     # dev2 配置，Windows x64/arm64 安装包
task pkg-win-dev3     # dev3 配置，Windows x64/arm64 安装包
task pkg-win-dev4     # dev4 配置，Windows x64/arm64 安装包
task pkg-win-cn       # 中国区，Windows x64/arm64 安装包
task pkg-win-global   # 全球版，Windows x64/arm64 安装包
task pkg-mac-dev1     # dev1 配置，macOS x64/arm64 安装包
task pkg-mac-dev2     # dev2 配置，macOS x64/arm64 安装包
task pkg-mac-dev3     # dev3 配置，macOS x64/arm64 安装包
task pkg-mac-dev4     # dev4 配置，macOS x64/arm64 安装包
task pkg-mac-cn       # 中国区，macOS x64/arm64 安装包
task pkg-mac-global   # 全球版，macOS x64/arm64 安装包
task release-cn:windows-amd64       # 完整验证后打包中国区 Windows x64（config_cn）；强制代码签名
task release-cn:windows-arm64       # 完整验证后打包中国区 Windows arm64（config_cn）；强制代码签名
task release-global:windows-amd64   # 完整验证后打包全球版 Windows x64（config_global）；强制代码签名
task release-global:windows-arm64   # 完整验证后打包全球版 Windows arm64（config_global）；强制代码签名
```

Windows 与 macOS 打包相互独立：`task pkg-win-<name>` 只构建 Windows x64/arm64，`task pkg-mac-<name>` 只构建 macOS x64/arm64（macOS 上构建产出 dmg，Windows 上构建 macOS 产出 zip）。

安装包输出到 `dist/desktop/`。正式发布（签名、公证、原生安装器）仍应在对应操作系统执行对应平台任务：Windows 用上表命令，macOS 用 `task release-cn:darwin-<arch>` / `task release-global:darwin-<arch>`。

Windows 发布签名使用 Certum Cloud SimplySign：

完整的软件安装、SimplySign Desktop 登录和获取指纹步骤、配置、命令、脚本说明、签名验证及排障见 [`docs/WINDOWS_SIGNING.md`](docs/WINDOWS_SIGNING.md)。

1. 将 `signing.config.example.json` 复制为 `signing.config.json`，填写 Windows SDK 中微软 `signtool.exe` 的完整路径。本地配置已加入 `.gitignore`，不会提交或打进安装包。
2. 在 SimplySign Desktop 中人工完成登录，打开证书详情并复制 40 位 SHA-1 指纹，然后执行对应平台任务（如 `task release-cn:windows-amd64`）。
3. electron-builder 真正执行第一次代码签名时，release 会在终端等待粘贴证书指纹。按 Enter 后，对应用、Go sidecar 和 NSIS 安装包执行 `signtool sign /v /fd sha256 /sha1 <指纹> /tr http://timestamp.sectigo.com /td sha256 <文件>`；后续文件复用同一指纹。指纹格式错误、证书不可用或签名失败时发布会直接失败。

`task pkg-win-dev1` / `task pkg-mac-dev1` 仍生成不强制签名的普通测试包，不读取发布签名配置。

如果只需要独立 Go 后端，可以执行：

```bash
go run ./tools/build -goos=windows -goarch=amd64 -name=client -dist=dist -cmd=./cmd/client
```

## 原生能力

工作目录支持 Electron 系统原生目录选择器，同时保留浏览器模式下的手工路径输入。新增文件选择、托盘、通知、剪贴板、深链接或自动更新能力时，必须遵循：

`Electron Main → preload 限定 API → Vue composable → 页面`

不要在 Vue Renderer 中启用 Node.js，也不要用 Electron IPC 重复实现已有 Go 业务接口。
