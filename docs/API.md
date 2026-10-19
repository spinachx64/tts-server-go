# API 接口与参数

默认地址为 `http://localhost:1233`。服务监听所有网卡，远程调用时将 `localhost` 替换为服务器地址。
可通过启动参数 `-port` 修改端口。

## 接口一览

| 方法 | 路径 | 请求格式 | 响应 | Token 验证 |
|---|---|---|---|---|
| POST | `/api/ra` | JSON | JSON 或音频二进制 | 是，服务设置 Token 时 |
| POST | `/api/azure` | SSML | 音频二进制 | 是，服务设置 Token 时 |
| GET | `/api/azure/voices` | 无 | 音色列表 JSON | 否 |
| POST | `/api/creation` | JSON | 音频二进制 | 是，服务设置 Token 时 |
| GET | `/api/creation/voices` | 无 | 音色列表 JSON | 否 |
| GET | `/api/legado` | URL 查询参数 | 阅读 APP 导入配置 JSON | 否 |
| GET | `/` | 无 | 网页及静态资源 | 否 |

表中方法是调用约定；只有 `/api/ra` 严格限制为 POST，其他旧接口没有严格限制方法。
最近实测的渠道状态见 [README](../README.md#渠道与接口状态)。注册了路由不代表上游接口始终可用。

## Edge：`POST /api/ra`

此项目使用纯 Go 实现，不需要安装 Python 或 edge-tts，也不会通过 SSH 调用其他项目。
旧的 `/api/ra` SSML 请求已改为 JSON；旧客户端需要更新，重新生成阅读 APP 配置即可。

请求：`POST /api/ra`，`Content-Type: application/json`。
默认返回 JSON，音频为 Base64；设置 `response_format: "audio"` 直接返回音频二进制。
服务监听所有网卡，默认端口 1233。连接超时和整个 `/api/ra` 请求超时均为 30 秒。
请求体最多 2 MiB；长文本自动分段，整个请求仍受 30 秒限制。

### Token 验证

支持由外部程序维护的 [SQLite 多用户 Token 表](TOKENS.md)。启用 `-token-db` 后，合成接口检查 Token 的 SHA-256、启用状态和有效期，数据库异常返回 503；不再使用共享 `-token`。主页不提供 Token 管理功能。

Token 是你自己设置的服务访问密码，没有固定值，不需要向微软申请。
例如启动命令中的值为 `my-tts-secret`：

```sh
./output/tts-server-go -port 1233 -token my-tts-secret
```

客户端发送 `Token: my-tts-secret`，必须与启动参数完全一致。
未设置 `-token` 或设置为空时不验证 Token；设置后 `/api/ra` 的所有操作都需要此请求头。
Azure 和 Creation 的合成接口也沿用该 Token；它们的音色列表和静态页面不验证 Token。
此值与代码中的微软客户端令牌 `Ocp-Apim-Subscription-Key` 无关。

### 请求参数

| JSON 字段 | 默认值 / 用途 |
|---|---|
| `text` | 要合成的 UTF-8 文本，与 `file` 二选一 |
| `file` | 上传文本对象，例如 `{"name":"script.txt","content":"你好"}`；不读取服务器路径 |
| `voice` | `en-US-EmmaMultilingualNeural`；中文可用 `zh-CN-XiaoxiaoNeural` |
| `rate` | `+0%`，例如 `+20%`、`-10%` |
| `volume` | `+0%`，例如 `-20%` |
| `pitch` | `+0Hz`，例如 `+5Hz`、`-10Hz`；单位不是百分比 |
| `proxy` | 未指定时使用服务进程的代理环境变量；可指定 HTTP/HTTPS 代理 URL |
| `format` | `audio-24khz-48kbitrate-mono-mp3`；另支持 96 kbps MP3 和 `webm-24khz-16bit-mono-opus` |
| `write_media` | 返回的音频文件名，默认 `audio.mp3` 或 `audio.webm`；不写入服务器文件 |
| `write_subtitles` | `false`；可设为 `true` 或文件名 `"hello.srt"`，返回 SRT 字幕；需要默认 48 kbps MP3 格式 |
| `response_format` | `json` 或 `audio`；`audio` 模式不能同时返回字幕 |
| `list_voices` | `true` 时返回 `{"voices":[...]}`，不执行合成 |
| `version` / `help` | `true` 时返回 Go 后端信息和参数列表，不是 Python edge-tts 的版本 |

`list_voices`、`version`、`help` 三者及合成输入互斥。`list_voices` 可同时设置 `proxy`。
文件名只能为普通文件名，不接受目录。WebM 不支持长文本分段拼接，请用 MP3。
字幕按词边界生成 SRT；语速、音量、音高使用带正负号的整数，也接受 `0%` / `0Hz`。
没有独立情绪、风格、声音克隆或自定义 SSML 参数。

### 合成 MP3 和 SRT

```sh
curl http://localhost:1233/api/ra \
  -H 'Content-Type: application/json' \
  -H 'Token: my-tts-secret' \
  --data '{"text":"你好，欢迎使用语音合成。","voice":"zh-CN-XiaoxiaoNeural","rate":"-10%","volume":"-20%","pitch":"+5Hz","write_media":"hello.mp3","write_subtitles":"hello.srt"}' \
  -o result.json
```

响应示例：

```json
{
  "audio": "<Base64 音频>",
  "encoding": "base64",
  "content_type": "audio/mpeg",
  "format": "audio-24khz-48kbitrate-mono-mp3",
  "media_name": "hello.mp3",
  "subtitles": "1\n00:00:00,050 --> 00:00:00,412\n你好\n\n",
  "subtitles_name": "hello.srt"
}
```

在客户端保存文件：

```sh
python3 - <<'PYCODE'
import base64, json
from pathlib import Path
r = json.loads(Path("result.json").read_text())
Path("hello.mp3").write_bytes(base64.b64decode(r["audio"]))
Path("hello.srt").write_text(r["subtitles"], encoding="utf-8")
PYCODE
```

直接下载音频：

```sh
curl http://localhost:1233/api/ra \
  -H 'Content-Type: application/json' -H 'Token: my-tts-secret' \
  --data '{"text":"你好","voice":"zh-CN-XiaoxiaoNeural","response_format":"audio"}' \
  -o hello.mp3
```

查询音色：

```sh
curl http://localhost:1233/api/ra \
  -H 'Content-Type: application/json' -H 'Token: my-tts-secret' \
  --data '{"list_voices":true}'
```

### 错误响应

错误返回 JSON `{"error":"原因"}`。常见状态码：400 参数错误、401 Token 错误、
405 方法错误、415 非 JSON 请求、502 上游失败、504 请求取消或内部超时；请求达到 30 秒时外层超时处理返回 503。

## Azure：`POST /api/azure`

此接口调用微软演示服务，最近实测可合成音频；它不是需要自行配置订阅密钥的正式 Azure Speech API。

### 请求头

| 字段 | 说明 |
|---|---|
| `Content-Type` | 建议 `application/ssml+xml`；Body 是 SSML 文本 |
| `Format` | 音频格式，例如 `audio-24khz-48kbitrate-mono-mp3`；必须由客户端提供 |
| `Token` | 设置启动参数 `-token` 后必须发送相同值 |

音色、语速、音量、音高、语言以及音色支持的风格和角色均放在 SSML 中。
具体能力取决于音色，上游会校验 SSML；此接口没有 Edge JSON 参数的默认值处理。

```sh
curl http://localhost:1233/api/azure \
  -H 'Content-Type: application/ssml+xml' \
  -H 'Format: audio-24khz-48kbitrate-mono-mp3' \
  -H 'Token: my-tts-secret' \
  --data-binary '<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="zh-CN"><voice name="zh-CN-XiaoxiaoNeural"><prosody rate="0%" pitch="+0Hz">你好。</prosody></voice></speak>' \
  -o azure.mp3
```

成功时响应音频二进制，MP3 的 `Content-Type` 为 `audio/mpeg`。请求处理超时为 30 秒。
旧接口失败时返回纯文本，常见状态码为 401（服务 Token 不匹配）、500（上游失败）和 503（超时）。

## 音色列表

- Edge：`POST /api/ra`，JSON `{"list_voices":true}`，响应为 `{"voices":[...]}`；设置服务 Token 后需要请求头 `Token`。
- Azure：`GET /api/azure/voices`，无需服务 Token，返回上游音色列表 JSON。
- Creation：`GET /api/creation/voices`，无需服务 Token，但目前因上游认证 401 失败，HTTP 服务返回 500。

```sh
curl http://localhost:1233/api/azure/voices
```

列表字段由上游提供，常见字段有 `Name`、`ShortName`、`Locale` 和 `Gender`。
选用音色时使用其 `ShortName`，例如 `zh-CN-XiaoxiaoNeural`；不要使用界面上的中文显示名。

## Creation：`POST /api/creation`

**目前不可用**：最近测试中上游认证返回 401，服务返回 500。保留路由和协议说明供维护使用。

请求头为 `Content-Type: application/json`，设置服务 Token 后还需要 `Token`。

| JSON 字段 | 类型 | 说明 |
|---|---|---|
| `text` | string | 要合成的文本 |
| `voiceName` | string | 音色名称 |
| `voiceId` | string | Creation 音色 ID，从对应列表获取 |
| `secondaryLocale` | string | 可选的二级语言区域 |
| `rate` | string | 语速百分比，例如 `0%`、`-10%` |
| `volume` | string | 音量百分比，例如 `0%`、`-20%` |
| `style` | string | 音色支持的风格，例如 `general` |
| `styleDegree` | string | 风格强度，例如 `1.0` |
| `role` | string | 音色支持的角色，例如 `default` |
| `format` | string | 音频格式，例如 `audio-24khz-48kbitrate-mono-mp3` |

此旧接口的字段是字符串，不使用 Edge 的参数名称：音色字段是 `voiceName`，不是 `voice`。
`rate` 和 `volume` 的当前解析范围为 -128 至 127，省略或无法解析时回退到 0。
此接口没有 `pitch`、`proxy`、字幕或 Base64 JSON 响应参数。

```json
{
  "text": "这是接口测试。",
  "voiceName": "zh-CN-XiaoxiaoNeural",
  "rate": "0%",
  "volume": "0%",
  "style": "general",
  "styleDegree": "1.0",
  "role": "default",
  "format": "audio-24khz-48kbitrate-mono-mp3"
}
```

成功时返回音频二进制；非法 JSON 返回 400，上游失败返回 500，错误信息为纯文本。
请求处理超时为 30 秒。

## 阅读 APP：`GET /api/legado`

生成阅读（Legado）APP 的朗读引擎导入配置，不在此接口执行语音合成。

| 查询参数 | 说明 |
|---|---|
| `api` | 目标合成接口的完整 URL，例如 `http://服务器IP:1233/api/ra` |
| `name` | 朗读引擎显示名称 |
| `voiceName` | 音色 ID |
| `voiceFormat` | 音频格式 |
| `token` | 写入导入配置的服务 Token，取值与目标服务的 `-token` 一致 |
| `concurrentRate` | 请求间隔，毫秒，例如 `5000` |
| `secondaryLocale` | Azure / Creation 的二级语言区域 |
| `styleName` | Azure / Creation 的风格 |
| `styleDegree` | Azure / Creation 的风格强度 |
| `roleName` | Azure / Creation 的角色 |
| `isCreation` | `1` 表示生成 Creation 配置；省略表示 Edge / Azure 配置 |
| `voiceId` | Creation 的音色 ID |

查询参数中的 `token` 只用于生成客户端配置，此路由本身不验证 Token。
目标为 `/api/ra` 时生成 JSON 请求，并设置 `response_format: "audio"`，方便阅读 APP 播放。
请求处理超时为 15 秒。

```sh
curl --get http://localhost:1233/api/legado \
  --data-urlencode 'api=http://localhost:1233/api/ra' \
  --data-urlencode 'name=晓晓' \
  --data-urlencode 'voiceName=zh-CN-XiaoxiaoNeural' \
  --data-urlencode 'voiceFormat=audio-24khz-48kbitrate-mono-mp3' \
  --data-urlencode 'token=my-tts-secret' \
  --data-urlencode 'concurrentRate=5000'
```

手机使用时，`api` 应填写手机可访问的服务器地址。
响应字段包括 `name`、`url`、`header`、`contentType`、`id`、`lastUpdateTime` 和 `concurrentRate`。
