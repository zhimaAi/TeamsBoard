---
name: goteams-db
description: 通过 GoTeams 本地数据库服务，在当前任务授权的 database_profile_id 范围内查看表结构、查询数据并执行受控写入。用于结合代码改动核对数据库表结构、验证数据，或根据需求对授权数据库执行受控写操作。
---

# GoTeams DB

必须使用 `scripts/db_api.py`，不要自行猜测服务地址、端口、用户名、密码或 database_profile_id。

## 运行约束

- `database_profile_id` 由任务创建时写入运行上下文 `[GoTeams Skill 运行上下文]` 区块，形如 `database=<库>, database_profile_id: <id>, db_type=<类型>`。**必须**使用该区块列出的 `database_profile_id`，不要使用 `1` 或其他任意数值。
- 脚本运行时由本地进程环境自动注入请求地址与访问令牌，无需手动指定。
- 仅可对运行上下文 `[GoTeams Skill 运行上下文]` 中列出的 `database_profile_id` 对应的数据库执行操作；服务端会拒绝越权访问。
- 查询前先运行 `tables` 核对库表，再运行 `structure` 查看字段；写入需 `--confirmed true`。
- 不执行 DROP / DELETE 等破坏性语句；脚本内置 SQL 白名单校验会拦截。

## 命令

在本 Skill 目录执行。`<database_profile_id>` 替换为运行上下文 `[GoTeams Skill 运行上下文]` 中列出的实际 id：

```bash
python scripts/db_api.py tables --database-profile-id <database_profile_id>
python scripts/db_api.py structure --database-profile-id <database_profile_id> --table <表名>
python scripts/db_api.py rows --database-profile-id <database_profile_id> --table <表名> --limit 100
python scripts/db_api.py query --database-profile-id <database_profile_id> --sql "SELECT * FROM <表名> LIMIT 10"
python scripts/db_api.py write --database-profile-id <database_profile_id> --sql "INSERT INTO <表名> ..." --confirmed true
```

运行 `python scripts/db_api.py <command> --help` 查看完整参数。

## 必需操作

### 核对表结构

运行 `tables` 取得库表清单；用 `structure` 查看目标表字段、类型与约束。

### 查询数据

运行 `rows` 或 `query`。`query` 接受任意 SELECT；结果以行列形式返回。

### 受控写入

运行 `write --confirmed true`。仅支持白名单内的 INSERT/UPDATE；DELETE/DROP 被拒绝。

