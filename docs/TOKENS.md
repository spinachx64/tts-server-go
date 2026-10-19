# SQLite Token 验证

Web 主页仍只填写访问 Token。服务不提供创建、分发、管理 Token 的页面或 API；由外部程序维护数据库，服务以只读方式查询。启用后，Edge、Azure 和 Creation 的合成接口统一验证；首页、音色列表及阅读配置生成仍公开。

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

分发程序生成高强度随机 Token，将原文交给用户，将哈希写入表。以下 Python 示例仅供分发程序参考，TTS 服务本身不执行：

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
