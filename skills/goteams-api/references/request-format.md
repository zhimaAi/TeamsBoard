# 请求定义参数

## 创建字段

| 参数 | JSON 类型 | 说明 |
| --- | --- | --- |
| `name` | string | 必填，接口名称 |
| `method` | string | 默认 `GET`，建议使用大写 |
| `url` | string | 必填，必须统一为 `{{Url}}/xx/xxx` 形式（见下方「URL 约定」），禁止传完整 URL 或裸路径 |
| `description` | string | 接口说明 |
| `headers` | object | 请求头，例如 `{"Content-Type":"application/json"}` |
| `query` | array | Query 参数项 |
| `auth` | object | 认证配置 |
| `body` | string | JSON/文本请求体的原始字符串 |
| `body_type` | string | `none`、`json`、`text`、`x-www-form-urlencoded`、`multipart` |
| `body_form` | array | 表单参数项 |
| `environment_id` | integer | 环境 ID，默认 `0` |

`collection_id` 和 `folder_id` 由脚本从任务环境强制注入，不接受跨范围值。

## URL 约定

所有创建的接口 URL 必须统一使用环境变量占位 `{{Url}}`，格式为 `{{Url}}/接口路径`，与具体 host 解耦，由执行时的环境（`environment_id`）解析：

- 来源是完整绝对 URL `http(s)://host[:port]/api/orders` → 剥离 `http(s)://host[:port]`，得到 `{{Url}}/api/orders`。
- 来源是裸路径 `/api/orders` 或 `api/orders` → 补 `{{Url}}` 前缀，得到 `{{Url}}/api/orders`。
- 已经以 `{{Url}}` 开头 → 原样保留。

```text
http://localhost:8080/api/orders   ->  {{Url}}/api/orders
/api/orders                        ->  {{Url}}/api/orders
{{Url}}/api/orders                 ->  {{Url}}/api/orders
```

## KeyValue

`query` 和 `body_form` 使用：

```json
[
  {"key": "page", "value": "1", "enabled": true},
  {"key": "avatar", "value": "C:\\tmp\\avatar.png", "enabled": true, "type": "file"}
]
```

`type=file` 仅用于 multipart 文件字段，其余项可省略 `type`。

## Auth

```json
{"type": "none"}
{"type": "bearer", "token": "secret"}
{"type": "basic", "username": "user", "password": "secret"}
{"type": "api-key", "key": "X-API-Key", "value": "secret", "add_to": "header"}
```

敏感值由 GoTeams SecretStore 保存，接口明细只返回 `has_secret`，不会回显明文。

## CLI JSON 参数

```bash
python scripts/goteams_api.py create \
  --name "分页查询订单" \
  --method GET \
  --url "{{Url}}/api/orders" \
  --headers '{"Accept":"application/json"}' \
  --query '[{"key":"page","value":"1","enabled":true}]'
```

PowerShell 中建议用单引号包裹 JSON。复杂更新可先构造紧凑单行 JSON，再传给 `--data`。
