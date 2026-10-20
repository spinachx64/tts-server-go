---
name: tts-cookup-macos
description: 在 macOS 上引导用户用普通 Token 调用 tts.cookup.cn 的 Edge 或 Azure 接口，将文本生成音频文件，适用于朗读、配音和工作流脚本。
---

# Cookup 语音合成 · macOS

让用户提供自己的普通用户 Token，确认基础参数后调用随包脚本生成音频。Edge 和 Azure 共用此 Skill，以 `--channel` 选择；不调用 Token 管理 API。

1. 定位本 Skill 下的 `scripts/tts_client.py`，检查 `python3 --version`（Python 3.9+，仅标准库）。若没有 Python，告知用户需要安装 Python 3，待可用后继续；不擅自修改系统 Python。
2. 先引导用户准备普通 Token，说明管理 Token 无法合成。优先让用户在自己的终端运行下面的交互命令，Token 将隐藏输入；已有 `TTS_TOKEN` 安全环境变量时直接复用。不要把 Token 写进命令参数、脚本、仓库、长期记忆或输出日志。
3. 确认渠道、文本或 UTF-8 文件、音色、语速、音量、音高、输出格式和保存位置。未指定时可使用默认 Edge、晓晓、0/0/0、MP3；询问缺失的文本和凭据即可，不反复询问已经给定的参数。按需读取 [参数与音色](references/parameters.md)。交互脚本按“Token → 参数 → 文本 → 输出路径”的顺序引导。
4. 用随包脚本执行；解析脚本的实际绝对路径，不假设用户工作目录。以下示例在 Skill 目录中运行：

```sh
python3 scripts/tts_client.py --interactive
```

已有安全环境变量时可自动化调用（参数来自用户确认）：

```sh
python3 scripts/tts_client.py --channel edge --text-file input.txt \
  --voice zh-CN-XiaoxiaoNeural --rate -10 --volume 0 --pitch 0 \
  --format mp3 --output narration.mp3
# Azure 使用完全相同参数，只将 --channel 改为 azure
```

脚本保留 UTF-8，正确区分 Edge JSON 与 Azure SSML，并验证音频响应。不要用 shell 字符串拼接用户文本；长文本通过 `--text-file` 传递。输出文件默认不覆盖；需要覆盖时先确认用户意图，再传 `--force`。

首次调用失败后脚本自动重试 3 次（最多 4 次请求，间隔 2/4/8 秒），包括网络错误、HTTP 错误和无效音频。仍失败就停止并告知最后的状态码/错误类别及可行修复，不再启动新一轮无限重试。401 请用户更正普通 Token；429 提示限流；不要换用他人 Token。失败时不交付错误响应伪装的音频。

成功后报告音频绝对路径、渠道和音色。用户需要工作流时，可把这条已验证调用整理为其工作流步骤，Token 继续从私有环境变量读取。不得扩展成部署或管理用户 Token 的任务。
