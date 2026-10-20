# 调用参数

服务地址固定为 `https://tts.cookup.cn`。普通用户 Token 通过 `Token` 请求头传递；`Authorization: Bearer` 是服务器管理 API 的凭据，不用于合成。此 Skill 不调用管理 API。

| 参数 | 默认 | 可选范围 |
|---|---|---|
| channel | edge | edge / azure |
| voice | zh-CN-XiaoxiaoNeural | 下表或用户提供的有效音色名 |
| rate | 0 | -50..100，百分比 |
| volume | 0 | -100..0，百分比 |
| pitch | 0 | -50..50，Hz |
| format | mp3 | Edge：mp3/webm；Azure：mp3/webm/wav |
| text / text-file | 必填 | 直接文本或 UTF-8 文本文件 |
| output | audio.mp3 | 输出文件路径；已有文件默认不覆盖 |

| 音色 | 参数 | 性别 |
|---|---|---|
| 晓晓 | zh-CN-XiaoxiaoNeural | 女 |
| 晓伊 | zh-CN-XiaoyiNeural | 女 |
| 云希 | zh-CN-YunxiNeural | 男 |
| 云健 | zh-CN-YunjianNeural | 男 |
| 云夏 | zh-CN-YunxiaNeural | 男 |
| 云扬 | zh-CN-YunyangNeural | 男 |

共享客户端为 Edge 发送 JSON 到 `/api/ra`，为 Azure 发送转义后的 UTF-8 SSML 到 `/api/azure` 并设置 `Format`；保存音频二进制，不把 JSON 错误保存成音频。服务单次请求约 30 秒超时，先使用短文本验证；长任务按用户要求分段处理，保留段落顺序。

每次合成在失败后重试 3 次，最多 4 次请求，等待 2/4/8 秒；网络异常、HTTP 错误和无效音频均计为失败。仍失败就停止，告知最后的 HTTP 错误码或错误类别，不再重复启动客户端重试。401 请用户检查普通 Token 是否正确、过期或被禁用；429 提示限流；5xx 提示上游或服务错误。不得切换、借用他人 Token 来绕过限制。
