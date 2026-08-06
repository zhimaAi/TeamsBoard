---
name: goteams-api
description: 通过 GoTeams 本地接口管理服务，在当前任务授权的接口集合和文件夹内列出、查看、创建、修改、删除和执行 API 请求定义。用于根据当前代码改动维护“接口开发”中的接口、查询文件夹内接口、查看接口明细，或把新增/变更接口同步到 GoTeams。
---

# GoTeams API

必须使用 `scripts/goteams_api.py`，不要自行猜测服务地址、端口、集合 ID、文件夹 ID 或请求 JSON。

## 运行约束

- 从环境变量读取 `GOTEAMS_LOCAL_BASE_URL`、`GOTEAMS_LOCAL_CAPABILITY_TOKEN`、`GOTEAMS_API_COLLECTION_ID`、`GOTEAMS_API_FOLDER_ID`。
- 只操作当前任务授权文件夹中的接口。服务端会拒绝跨集合、跨文件夹访问。
- 不创建、修改或删除集合、文件夹、环境。
- 修改或删除前先运行 `get` 核对目标。
- 根据代码改动添加接口时，先搜索控制器/路由/DTO/请求模型，确认 Method、URL、参数、认证和请求体。
- **接口 URL 必须统一使用环境变量的 `{{Url}}/xx/xxx` 约定**：`--url` 永远传 `{{Url}}/接口路径`，不要传完整 URL 或裸路径。来源是 `http(s)://host/path` 时，先剥离 host 再补 `{{Url}}` 前缀；已经是 `{{Url}}/...` 则原样保留。
- 将结构化结果保留为 JSON；脚本失败时根据 stderr 的 HTTP 状态和错误修正参数。

## 命令

在本 Skill 目录执行：

```bash
python scripts/goteams_api.py context
python scripts/goteams_api.py list
python scripts/goteams_api.py get 123
python scripts/goteams_api.py create --name "创建订单" --method POST --url "{{Url}}/api/orders" --body-type json --body '{"product_id":1}'
python scripts/goteams_api.py update 123 --data '{"name":"更新订单","method":"PUT","url":"{{Url}}/api/orders/{id}"}'
python scripts/goteams_api.py delete 123
python scripts/goteams_api.py execute 123
python scripts/goteams_api.py history 123 --limit 20
```

运行 `python scripts/goteams_api.py <command> --help` 查看完整参数。

创建和修改复杂请求前读取 [references/request-format.md](references/request-format.md)。

## 必需操作

### 列出文件夹内接口

运行 `list`。脚本固定携带任务集合和文件夹；不要改用集合级列表绕过目录过滤。

### 查看接口明细

运行 `get <id>`。目标不在任务文件夹时服务端返回 403。

### 创建接口

运行 `create`。至少传 `--name`、`--method`、`--url`。集合和文件夹由脚本强制注入。

### 修改接口

先运行 `get`，再运行 `update <id> --data '<JSON对象>'`。只传需要修改的字段。

### 删除接口

先运行 `get` 确认名称、Method 和 URL，再运行 `delete <id>`。

