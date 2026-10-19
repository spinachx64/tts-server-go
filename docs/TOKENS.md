# SQLite Token 验证

Web 主页仍只填写访问 Token。网页不提供创建或管理功能；外部程序可直接维护数据库，也可通过专用管理 API 创建、查询和删除 Token。认证查询以只读方式进行。启用后，Edge、Azure 和 Creation 的合成接口统一验证；首页、音色列表及阅读配置生成仍公开。

## 表结构与约定

完整建表 SQL：[schema.sql](../internal/tokenstore/schema.sql)。分发程序先创建数据库及表：

```sh
sqlite3 /var/lib/tts-server-go/tokens.db < internal/tokenstore/schema.sql
```

| 字段 | 含义 |
|---|---|
| `token_hash` | Token 原文的 SHA-256，小写十六进制，64 字符，主键 |
| `user_id` | 分发程序的用户标识，一个用户可有多个 Token |
| `enabled` | 1 有效，0 禁用 |
| `expires_at` | UTC Unix 秒，NULL 永不过期；等于或小于当前时间即失效 |
| `created_at` | 创建时间，默认当前 UTC Unix 秒 |

分发程序生成高强度随机 Token，将原文交给用户，将哈希写入表。以下 Python 示例仅供分发程序参考，手动接入示例：

```python
import hashlib, secrets, sqlite3

token = secrets.token_urlsafe(32)
with sqlite3.connect('/var/lib/tts-server-go/tokens.db') as db:
    db.execute('INSERT INTO tokens(token_hash,user_id) VALUES (?,?)',
               (hashlib.sha256(token.encode()).hexdigest(), 'user-001'))
# 通过你的分发渠道将 token 原文交给用户，避免记录到日志。
```

禁用、删除或修改有效期，后续请求立即生效，无需重启。不要在运行时替换数据库文件，应在原数据库中事务更新。数据库不可用时返回 503，错误、缺失、禁用、过期 Token 返回 401。空表会拒绝全部合成请求。

## 启动

```sh
./output/tts-server-go -port 1233 -token-db /var/lib/tts-server-go/tokens.db
```

SQLite 模式支持当前服务器 Linux amd64 及驱动支持的常见桌面/服务器平台；Android 和 Linux MIPS 构建保留原单 Token 模式，指定 `-token-db` 会启动失败。

`-token-db` 和旧的 `-token` 互斥，避免旧共享 Token 绕过验证。两者都不设置则保持旧行为：不验证。

服务器使用 systemd 时，在 `/etc/tts-server-go.env` 设置：

```ini
TTS_PORT=1233
TTS_TOKEN=
TTS_TOKEN_DB=/var/lib/tts-server-go/tokens.db
```

使用更新后的 `deploy/tts-server-go.service`，执行 `systemctl daemon-reload` 后重启服务。数据库不放在 releases/current 下，部署不会覆盖。分发程序拥有写权限，运行用户 `tts` 需要数据库和父目录的读/遍历权限；WAL 模式下还需能读取 `-wal`、`-shm` 文件（由分发程序保持存在，使用兼容的权限）。不要将数据库放入网页静态目录或提交到仓库。

迁移时，如需保留旧 Token，由分发程序先将旧值的 SHA-256 导入表，再清空 `TTS_TOKEN` 并启用 `TTS_TOKEN_DB`。服务不会自动导入或生成 Token。

## 服务间 Token 管理 API

设置至少 32 字节的随机 `TTS_ADMIN_TOKEN` 环境变量（也支持 `-admin-token`）并启用 `-token-db` 后，提供以下管理接口。未配置时接口返回 404。管理 Token 不写入用户表，不可用于语音合成；普通用户 Token 也不能管理其它 Token。网页没有管理功能。

生产环境在 `/etc/tts-server-go.env` 配置 `TTS_ADMIN_TOKEN`，不要把真实值写进源码、网页配置或调用示例。修改后重启服务。

所有管理请求使用：

```http
Authorization: Bearer <管理Token>
```

| 方法 | 路径 | 请求内容 | 成功响应 |
|---|---|---|---|
| POST | `/api/tokens` | JSON：`username`、`expires_at` | 201：`token`、`username`、`expires_at`、`enabled` |
| GET | `/api/tokens` | `Token` 请求头传待查用户 Token | 200：`username`、`expires_at`、`enabled` |
| DELETE | `/api/tokens` | `Token` 请求头传待删用户 Token | 204，无响应正文 |

`expires_at` 使用 RFC3339（如 `2125-10-19T16:14:41+08:00`），创建时必填且必须为未来时间；响应统一为 UTC。旧数据库中永不过期的记录查询时返回 `null`。用户名与数据库 `user_id` 对应，去除首尾空白，最多 128 字节。同一用户可创建多个 Token。创建时生成随机 Token，只在该次响应中返回原文；后续查询不返回 Token。删除永久移除该条 Token，随后不能再访问合成接口。

macOS / Linux 示例（Windows 的其它服务按相同 HTTP 协议调用）：

```sh
ADMIN_TOKEN='替换为管理Token'
# 创建
curl --fail-with-body https://tts.cookup.cn/api/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"username":"user-003","expires_at":"2125-10-19T16:14:41+08:00"}'

USER_TOKEN='替换为创建响应中的token'
# 查询用户名、截止时间、启用状态（包含过期或禁用记录）
curl --fail-with-body https://tts.cookup.cn/api/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Token: $USER_TOKEN"
# 删除
curl --fail-with-body -X DELETE https://tts.cookup.cn/api/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Token: $USER_TOKEN"
```

错误：400 参数无效、401 管理认证失败、404 未启用或用户 Token 不存在、405 方法不支持、415 Content-Type 不正确、503 数据库暂不可用。所有响应设置 `Cache-Control: no-store`。管理请求日志的用户名为 `admin`（认证失败为 `unknown`），不记录认证请求头和返回的新 Token。

启用管理接口后，服务需要 Token 数据库及其父目录的写权限，以创建 SQLite 事务日志；systemd 配置使用 `ReadWritePaths=/var/lib/tts-server-go`。认证查询仍使用独立只读连接。只使用外部分发程序时可以不配置管理 Token，继续只读验证。

当前服务器的管理凭据存于 `/opt/project-config/tts-server-go/admin-token`，可通过以下命令查看：

```sh
ssh cook 'sudo cat /opt/project-config/tts-server-go/admin-token'
```

统一配置目录按 `/opt/project-config/<项目名>/` 分隔，目录权限 `0700`、凭据文件 `0600`，由 root 管理。运行环境中的 `TTS_ADMIN_TOKEN` 仍配置在 `/etc/tts-server-go.env`；轮换时需要同步更新并重启服务。不要将服务器凭据目录提交到 GitHub。
