# 图标资源索引

新增或导出图标前，先检查本索引和当前目录中的现有资源，图形与语义匹配时优先复用。

| 文件 | 图形描述 | 业务语义 | 推荐复用场景 | 当前使用位置 |
| --- | --- | --- | --- | --- |
| [`common-more-actions.svg`](./common-more-actions.svg) | 横向排列的三个实心圆点 | 更多操作、打开操作菜单 | 列表项、卡片等内容区域的更多操作按钮 | 任务会话列表、流水线列表 |
| [`agent-cli-reload.svg`](./agent-cli-reload.svg) | 圆形箭头（刷新） | 重新检测本机 CLI 运行状态 | Agent 编辑弹窗 CLI 行的重新检测按钮 | AgentEditorModal |
| [`task-documents-open-directory.svg`](./task-documents-open-directory.svg) | 文件夹内带向右箭头 | 在系统文件夹中打开任务目录 | 产出文档抽屉标题栏 | TaskDocumentsDrawer |
| [`task-documents-toggle-directory.svg`](./task-documents-toggle-directory.svg) | 面板矩形内带竖线和向左箭头 | 收起或展开产出文档目录 | 产出文档内容区工具栏左侧 | TaskDocumentsDrawer |
| [`task-composer-cli.svg`](./task-composer-cli.svg) | 终端窗口内带命令提示符 | 打开 CLI / 模型选择器 | 任务会话输入框左侧触发按钮 | ChatComposer |
| [`cli-picker-check.svg`](./cli-picker-check.svg) | 对勾 | 当前已选中的模型 | CLI / 模型双栏选择弹层的选中态 | CliRuntimePicker |
| [`batch-config.svg`](./batch-config.svg) | 三条横线旁带圆点（灰色） | 进入批量配置 CLI | Agent 编排页头批量配置按钮 | PipelineStepsPane / ExpertGroupsWorkspace |
| [`batch-cancel.svg`](./batch-cancel.svg) | 三条横线旁带圆点 | 退出批量配置 CLI | Agent 编排页头取消批量配置按钮 | PipelineStepsPane / ExpertGroupsWorkspace |
| [`batch-selected-count.svg`](./batch-selected-count.svg) | 圆圈内对勾 | 已选 Agent 数量 | 批量配置底部操作条计数 | AgentBatchConfigBar |
| [`codex-logo.svg`](./codex-logo.svg) | Codex 品牌标识（紫蓝渐变） | 标识 Codex 应用 | Codex 应用入口、Codex 任务提示 | CreateLocalTaskModal、AssignExecutionModeModal、TaskDetail、VibeCodingConversationNotice |
| [`vibe-codex-cli.svg`](./vibe-codex-cli.svg) | Codex 官方单色标识（近黑 #0D0D0D） | 标识 Codex CLI，与 Codex 应用的彩色标识区分 | Vibe Coding 的 Codex CLI 选项和对应任务头像 | useVibeCoding |
| [`vibe-coding-logo.svg`](./vibe-coding-logo.svg) | 星光加圆角方框（深灰 #595959） | Vibe Coding 执行方式标识 | Vibe Coding 执行方式入口、浅色背景上的图标 | CreateLocalTaskModal、AssignExecutionModeModal |
| [`vibe-coding-logo-blue.svg`](./vibe-coding-logo-blue.svg) | 星光加圆角方框（蓝 #318CE2） | Vibe Coding 执行方式标识（蓝色变体） | 蓝底容器中的 Vibe Coding 标识 | VibeCodingConversationNotice |
| [`task-switch-execution-mode.svg`](./task-switch-execution-mode.svg) | 双向循环箭头（灰 #595959） | 切换任务执行方式 | 任务详情执行方式切换按钮 | TaskDetail |
| [`team-login-feature-sync.svg`](./team-login-feature-sync.svg) | 方框对角带外扩箭头（蓝 #5B8DEF） | 团队连接能力：任务同步 | 团队工作登录引导卡片的能力标签 | TeamWorkLoginCard |
| [`team-login-feature-collaborate.svg`](./team-login-feature-collaborate.svg) | 两个并排人像（蓝 #5B8DEF） | 团队连接能力：团队协作 | 团队工作登录引导卡片的能力标签 | TeamWorkLoginCard |
| [`team-login-feature-agent.svg`](./team-login-feature-agent.svg) | 带天线的机器人头部（蓝 #5B8DEF） | 团队连接能力：Agent 协同 | 团队工作登录引导卡片的能力标签 | TeamWorkLoginCard |
| [`onboarding-spark.svg`](./onboarding-spark.svg) | 四角星光（#262626） | 新手指引开场标题标记 | 新增对话新手指引的开场卡片 | OnboardingGuide |
| [`sidebar-nav-light-new-conversation.svg`](./sidebar-nav-light-new-conversation.svg) | 方框加笔（#262626） | 侧栏「新对话」 | 浅色侧栏主导航 | AppSidebar |
| [`sidebar-nav-dark-new-conversation.svg`](./sidebar-nav-dark-new-conversation.svg) | 方框加笔（#94A3B8） | 侧栏「新对话」 | 深色侧栏主导航，图形与浅色稿相同 | AppSidebar |
