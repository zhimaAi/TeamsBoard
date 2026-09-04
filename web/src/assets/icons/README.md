# 图标资源索引

新增或导出图标前，先检查本索引和当前目录中的现有资源，图形与语义匹配时优先复用。

| 文件 | 图形描述 | 业务语义 | 推荐复用场景 | 当前使用位置 |
| --- | --- | --- | --- | --- |
| [`common-more-actions.svg`](./common-more-actions.svg) | 横向排列的三个实心圆点 | 更多操作、打开操作菜单 | 列表项、卡片等内容区域的更多操作按钮 | 任务会话列表、流水线列表 |
| [`agent-cli-reload.svg`](./agent-cli-reload.svg) | 圆形箭头（刷新） | 重新检测本机 CLI 运行状态 | Agent 编辑弹窗 CLI 行的重新检测按钮 | AgentEditorModal |
| [`task-documents-open-directory.svg`](./task-documents-open-directory.svg) | 文件夹内带向右箭头 | 在系统文件夹中打开任务目录 | 产出文档抽屉标题栏 | TaskDocumentsDrawer |
| [`task-composer-cli.svg`](./task-composer-cli.svg) | 终端窗口内带命令提示符 | 打开 CLI / 模型选择器 | 任务会话输入框左侧触发按钮 | ChatComposer |
| [`cli-picker-check.svg`](./cli-picker-check.svg) | 对勾 | 当前已选中的模型 | CLI / 模型双栏选择弹层的选中态 | CliRuntimePicker |
| [`batch-cancel.svg`](./batch-cancel.svg) | 三条横线旁带圆点 | 退出批量配置 CLI | Agent 编排页头取消批量配置按钮 | PipelineStepsPane |
| [`batch-selected-count.svg`](./batch-selected-count.svg) | 圆圈内对勾 | 已选 Agent 数量 | 批量配置底部操作条计数 | PipelineStepsPane |
