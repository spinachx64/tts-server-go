# 离线 Token 管理工具

`tools/token_admin.py` 仅使用 Python 3 标准库，直接操作 SQLite，不调用任何 API。默认数据库 `/opt/tts-server-go/data/tokens.db`，也可在子命令之前传 `--db /其他数据库路径`。工具生成的记录与服务读取同一张表，修改后续请求立即生效；无需重启服务。

服务器已安装到 `/opt/tts-server-go/tools/token_admin.py`。在 cook 上执行：

```sh
TOOL=/opt/tts-server-go/tools/token_admin.py
python3 "$TOOL" --help
python3 "$TOOL" list
```

需要对数据库及其目录有写权限；cook 上可使用 root 执行。脚本不创建管理 API 凭据，也不与管理 API 交换数据。

## 初始化与备注字段

```sh
python3 "$TOOL" init
```

新数据库创建 `tokens` 表；旧表仅增加 `note TEXT NOT NULL DEFAULT ''`，保留原有 Token、启用状态、创建和截止时间。重复执行安全。首次迁移前可用 SQLite backup 保存快照。已有数据库文件所有者和权限不变；新建数据库默认 0600，在 root 执行时沿用父目录所有者。

备注由此工具维护，现有管理 API 创建的记录默认为空备注；当前 API 请求字段保持不变。

## 批量生成

为不同用户生成 100 个 Token，有效期 30 天：

```sh
python3 "$TOOL" create --prefix member- --start 1 --count 100 --days 30 \
  --note '2026年10月批次' \
  --out /opt/project-config/tts-server-go/member-october.json
```

用户名为 `member-0001`、`member-0002`……。同一用户名需要多个 Token 时，用 `--user alice --count 3` 替代 `--prefix`。用户名可重复，每个 Token 独立管理。

也可指定带时区的截止时间或永不过期：

```sh
python3 "$TOOL" create --user alice --expires-at '2125-10-19T16:14:41+08:00' \
  --note '长期用户' --out /opt/project-config/tts-server-go/alice.json
# --never 表示永不过期；不传有效期选项默认 365 天
```

`--out` 必填，可选 `.json` 或 `.csv`。导出文件包含用户名、Token 原文、Token 编号、有效期及备注，权限 0600；已有文件不会被覆盖。终端只打印生成数量和保存路径，不打印原文。数据库仅保存 SHA-256 哈希，无法从数据库取回原文，因此请保存好导出文件。输出文件写入失败时本批数据库插入回滚。

## 查询、续期及备注

```sh
python3 "$TOOL" list
python3 "$TOOL" list --user alice
python3 "$TOOL" list --note '长期'
python3 "$TOOL" show --id 数据库中token_id的前12位

# 活跃 Token 从原截止时间延长；过期 Token 从当前时间延长
python3 "$TOOL" renew --id 数据库中token_id的前12位 --days 30
# 精确设置截止时间
python3 "$TOOL" renew --user alice --expires-at '2126-10-19T16:14:41+08:00'
python3 "$TOOL" note --id 数据库中token_id的前12位 --text '已续期，联系人：张先生'
# 清空备注：--text ''
```

`--id` 使用完整 `token_id` 或至少 8 位唯一哈希前缀，有歧义会拒绝；`--user` 精确匹配并操作该用户名下的所有 Token。也支持 `--token-file` 指向只包含一个 Token 原文的文件。永不过期记录不接受 `renew --days`，可用 `--expires-at` 明确设为有限有效期，或 `--never` 保持永不过期。续期不会更换 Token，也不会自动启用已禁用的 Token。

## 启用、禁用、删除

```sh
python3 "$TOOL" disable --user alice
python3 "$TOOL" enable --id 数据库中token_id的前12位
python3 "$TOOL" delete --id 数据库中token_id的前12位
```

删除操作永久移除选中的记录；`--user` 会删除该用户全部 Token。工具使用事务和 SQLite 锁，避免批量操作与其它写入互相覆盖。

更新服务器脚本时，从本机仓库上传：

```sh
scp tools/token_admin.py cook:/opt/tts-server-go/tools/token_admin.py
```
