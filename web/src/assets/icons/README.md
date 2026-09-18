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
| [`batch-cancel.svg`](./batch-cancel.svg) | 三条横线旁带圆点 | 退出批量配置 CLI | Agent 编排页头取消批量配置按钮 | PipelineStepsPane |
| [`batch-selected-count.svg`](./batch-selected-count.svg) | 圆圈内对勾 | 已选 Agent 数量 | 批量配置底部操作条计数 | PipelineStepsPane |
| [`codex-logo.svg`](./codex-logo.svg) | Codex 品牌标识（紫蓝渐变） | 标识 Codex 执行方式 | Codex 执行方式入口、Codex 任务提示 | CreateLocalTaskModal、AssignExecutionModeModal、TaskDetail、VibeCodingConversationNotice |
| [`vibe-coding-logo.svg`](./vibe-coding-logo.svg) | 星光加圆角方框（深灰 #595959） | Vibe Coding 执行方式标识 | Vibe Coding 执行方式入口、浅色背景上的图标 | CreateLocalTaskModal、AssignExecutionModeModal |
| [`vibe-coding-logo-blue.svg`](./vibe-coding-logo-blue.svg) | 星光加圆角方框（蓝 #318CE2） | Vibe Coding 执行方式标识（蓝色变体） | 蓝底容器中的 Vibe Coding 标识 | VibeCodingConversationNotice |
| [`task-switch-execution-mode.svg`](./task-switch-execution-mode.svg) | 双向循环箭头（灰 #595959） | 切换任务执行方式 | 任务详情执行方式切换按钮 | TaskDetail |
