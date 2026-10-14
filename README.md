# tts-server-go

基于 Go 的 HTTP 文本转语音服务，调用微软在线语音接口，提供网页试听、音色选择和阅读（Legado）APP 配置导入。
Edge 渠道使用纯 Go 实现，支持语速、音量、音高、代理、长文本分段及 SRT 字幕，无需安装 Python 或 edge-tts。

完整的请求参数、响应格式和调用示例见 **[API 接口文档](docs/API.md)**。

## 渠道与接口状态

以下为 **2026-10-13** 在本机和 cook 独立测试实例上的结果，后续可用性以在线检测为准。

| 渠道 | 合成接口 | 输入协议 | 音频合成 | 音色列表 |
|---|---|---|---|---|
| Edge | `/api/ra` | JSON | 可用，MP3 / WebM；MP3 可返回 SRT | 可用，通过 `/api/ra` 查询 |
| Azure 演示服务 | `/api/azure` | SSML | 可用 | `/api/azure/voices` 可用 |
| Creation / Speech Studio | `/api/creation` | JSON | 不可用，上游认证 401 | `/api/creation/voices` 同样认证失败 |

Edge 测试中曾出现微软 429 限流，单独复测成功。Creation 路由保留，暂不建议用于正常合成。
服务另外提供 `/api/legado` 生成阅读 APP 配置，`/` 为网页配置界面。

## 构建与启动

需要 Go 1.26.1 或更高版本，合成时需要联网。

```sh
go build -o output/tts-server-go ./cmd/cli
./output/tts-server-go -port 1233 -token my-tts-secret
```

也可以直接运行源码：

```sh
go run ./cmd/cli -port 1233 -token my-tts-secret
```

打开 `http://localhost:1233`，在同一个语音工作台切换 Edge / Azure，选择音色、试听、下载音频或生成阅读 APP 导入配置。
Azure 的风格、角色和朗读语言根据音色动态显示，Edge 支持 SRT 字幕。旧地址 `/azure.html` 会进入工作台的 Azure 渠道。
服务监听所有网卡，其他设备使用 `http://服务器IP:1233` 访问。

| 启动参数 | 默认值 | 说明 |
|---|---|---|
| `-port` | `1233` | HTTP 服务端口 |
| `-token` | 空 | 自定义访问令牌；空值表示不验证 |
| `-use-dns-edge` | `false` | 保留兼容旧命令；Edge 始终使用 DNS，取值不影响连接 |

Token 是你自己设置的服务访问密码，没有固定值，也不需要向微软申请。
例如使用 `-token my-tts-secret` 启动，客户端就发送请求头 `Token: my-tts-secret`。
它与微软接口的内部客户端令牌无关，验证范围见 [Token 说明](docs/API.md#token-验证)。

## Edge 快速调用

直接下载 MP3：

```sh
curl http://localhost:1233/api/ra \
  -H 'Content-Type: application/json' \
  -H 'Token: my-tts-secret' \
  --data '{"text":"你好，欢迎使用语音合成。","voice":"zh-CN-XiaoxiaoNeural","rate":"-10%","response_format":"audio"}' \
  -o hello.mp3
```

需要音频和字幕时，使用默认的 JSON 响应：

```sh
curl http://localhost:1233/api/ra \
  -H 'Content-Type: application/json' \
  -H 'Token: my-tts-secret' \
  --data '{"text":"你好。","voice":"zh-CN-XiaoxiaoNeural","write_subtitles":"hello.srt"}' \
  -o result.json
```

响应的 `audio` 为 Base64 音频，`subtitles` 为 SRT 文本。解码、文件输入、代理和音色查询示例见 [API 文档](docs/API.md)。

## 自动构建与部署

GitHub Actions 工作流 **Build and deploy to cook** 在 `master` 的源码等变更后自动测试、构建 Linux amd64 二进制，配置部署 Secrets 后通过 SSH 上传到 cook 并重启 systemd 服务。支持手动触发、SHA-256 校验、健康检查和失败回滚。

服务器初始化、Token 设置、SSH 密钥以及 Secrets 清单见 **[部署文档](docs/DEPLOY.md)**。服务器不需要安装 Go、Python 或 edge-tts。

## 兼容性说明

- `/api/ra` 已由 SSML 改为 JSON，请发送 `Content-Type: application/json`，音频格式放在 JSON 的 `format` 字段中。
- 默认响应是 JSON；网页和阅读 APP 使用 `response_format: "audio"` 播放音频。旧版阅读配置需要重新生成并导入。
- Azure 仍接收 SSML 和 `Format` 请求头，Creation 仍使用原有 JSON 协议。
- Edge 连接超时和整个请求处理超时均为 30 秒。长文本自动分段，但仍受整个请求的超时限制。
- Edge 合成地址已参考 [edge-tts 修复提案 #491](https://github.com/rany2/edge-tts/pull/491) 切换至 `api.msedgeservices.com`，包含 GEC 校验、MUID 和 403 校时重试，不再连接旧固定 IP。

## 检查与测试

离线测试及静态检查：

```sh
go test -timeout 30s ./...
go vet ./...
go test -race -timeout 30s ./server ./tts/edge
# 前端渠道切换和请求协议检查（需要 Node.js）
node scripts/test_studio.js
```

启动服务后，运行在线 API 检测脚本（需要 Python 3 标准库）：

```sh
python3 scripts/test_apis.py --url http://localhost:1233 --token my-tts-secret
# 同时验证文件输入、语速/音量/音高、音频响应和长文本字幕
python3 scripts/test_apis.py --url http://localhost:1233 --token my-tts-secret --extended
```

结果默认保存到 `output/api-test/report.json`，原始响应、成功的 MP3 和 SRT 也会保存。
使用 `--output` 指定目录；`--edge-protocol ssml` 可用于对比迁移前的旧服务。
脚本逐项检查各渠道的合成与音色列表，任一接口失败时退出码为 1。当前 Creation 不可用，因此完整检测通常返回 1，请查看各项结果。

直接运行包内的在线接口测试：

```sh
TTS_INTEGRATION_TEST=1 go test -timeout 60s ./tts/edge ./tts/azure ./tts/creation
```

在线测试可能因上游协议、限流或网络而失败；编译与离线测试通过不代表在线渠道可用。

## 上游与相关项目

- 原项目：[jing332/tts-server-go](https://github.com/jing332/tts-server-go)
- 原项目发布包：[Releases](https://github.com/jing332/tts-server-go/releases)；最新构建：[Actions](https://github.com/jing332/tts-server-go/actions)
- 安卓版：[tts-server-android](https://github.com/jing332/tts-server-android)
- Edge 协议参考：[rany2/edge-tts](https://github.com/rany2/edge-tts)

原项目标注为 MIT 许可证。
