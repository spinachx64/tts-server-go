# API 调用日志

API 请求按 JSON Lines 写入文件，每次调用一行。网页和静态文件访问不记录。

- 本机默认 `logs/api-YYYY-MM-DD.jsonl`，使用 `-log-dir` 或环境变量 `TTS_LOG_DIR` 修改目录（启动参数优先）。
- 部署的 systemd 服务使用 `/opt/tts-server-go/logs/api-YYYY-MM-DD.jsonl`，按北京时间切日，日志时间包含 `+08:00`。
- 按请求开始日期归档，跨零点完成的请求仍写入开始当天的文件。
- 目录由 systemd 创建并授权给运行用户 `tts`；文件权限 `0600`，部署更新不会覆盖。
- 每次追加后关闭文件，可直接移动或删除旧日志回收；服务不会自动删除历史文件。若删除当天日志，下一次请求会重新创建。

字段：

| 字段 | 内容 |
|---|---|
| `time` / `completed_at` | 请求开始和完成时间 |
| `username` | SQLite `tokens.user_id`；未知 Token 为 `unknown`；旧共享 Token 为 `shared`；未配置认证为 `anonymous`；管理接口认证成功为 `admin` |
| `method` / `path` / `query` | 请求方法、API 路径、查询参数（`token` 参数脱敏） |
| `request_body` | 上传的 JSON、SSML 或其他请求正文，换行在 JSON 中转义 |
| `success` | HTTP 2xx/3xx 为 true，其余为 false |
| `status_code` | HTTP 状态码 |
| `error_code` / `error` | 失败时记录 HTTP 错误码与错误响应前 4 KiB（超时同样记录） |
| `response_bytes` / `duration_ms` | 响应字节数及总耗时 |

不记录 Token 请求头或音频响应。请求正文包含用户上传的原文，日志应按业务需要保存和回收。已过期/禁用但仍在表内的 Token，日志仍能关联其用户名。

查看日志：

```sh
ssh cook 'sudo tail -n 20 /opt/tts-server-go/logs/api-$(date +%F).jsonl'
```

日志目录不可写时服务启动失败；运行时写日志失败会写入 systemd journal，不影响 API 响应。
