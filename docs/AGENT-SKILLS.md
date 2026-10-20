# 可下载 Agent Skill

网页“API 调用示例”中提供两套下载：

- `/downloads/tts-cookup-macos.zip`
- `/downloads/tts-cookup-windows.zip`

每个包都是完整 Skill，包含 `SKILL.md`、`scripts/tts_client.py` 和 `references/parameters.md`。解压后交给 agent 阅读，或安装到该 agent 的技能目录。两个包共用一份 Python 客户端与参数说明，都支持 Edge/Azure，无需按渠道重复下载。需要 Python 3.9+，无需额外 Python 依赖。

Agent 引导用户先准备普通用户 Token，再配置渠道、文本、音色、语速、音量、音高、格式和输出位置。交互模式隐藏输入 Token；自动化可从安全环境变量 `TTS_TOKEN` 读取。凭据不包含在下载包中，也不保存到脚本或仓库。

```sh
# macOS，在解压后的 Skill 目录执行
python3 scripts/tts_client.py --interactive
```

```powershell
# Windows，在解压后的 Skill 目录执行
py -3 .\scripts\tts_client.py --interactive
```

首次请求失败后最多重试 3 次，总共最多 4 次请求，退避 2/4/8 秒。网络失败、HTTP 错误、空/无效音频响应均触发重试。耗尽后停止并返回非零退出码；成功时才保存完整音频。默认不覆盖已有文件，明确需要时使用 `--force`。

源文件位于 `skills/macos`、`skills/windows` 与 `skills/common`。构建下载包：

```sh
python3 scripts/package_skills.py
python3 scripts/package_skills.py --check
python3 -m unittest discover -s tests -p 'test_*.py'
```

归档以固定时间戳、权限及不压缩 ZIP 生成，保证跨平台构建一致。打包时复制共享客户端，使两份下载各自独立可用。ZIP 位于 `server/public/downloads/`，由 Go 嵌入并随正常发布上线；CI 检查源文件与 ZIP 是否同步。
