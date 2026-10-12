![MIT](https://img.shields.io/badge/license-MIT-green)
[![CI](https://github.com/jing332/tts-server-go/actions/workflows/main.yml/badge.svg)](https://github.com/jing332/tts-server-go/actions/workflows/main.yml)
[![Test](https://github.com/jing332/tts-server-go/actions/workflows/test.yml/badge.svg)](https://github.com/jing332/tts-server-go/actions/workflows/test.yml)
![GitHub release (latest by date)](https://img.shields.io/github/downloads/jing332/tts-server-go/latest/total)

# 安卓系统推荐使用 tts-server-android 安装即用
https://github.com/jing332/tts-server-android

# 下载
请从 [Release](https://github.com/jing332/tts-server-go/releases) 下载稳定版。

或者从 [Actions](https://github.com/jing332/tts-server-go/actions) 下载最新代码构建程序。

# 使用方法
直接运行，默认监听1233端口。

然后浏览器 `http://localhost:1233` 打开网页，选好配置导入阅读后即可朗读。

# 提示

接口与[ms-ra-forwarder](https://github.com/wxxxcxx/ms-ra-forwarder) 相同:

微软Azure接口(延迟高): `http://localhost:1233/api/azure`

Edge大声朗读接口: `http://localhost:1233/api/ra`

## 本地开发（Go 1.26.1）

当前项目已按 Go 1.26.1（macOS arm64）完成构建和本地测试兼容调整。

```sh
go build -o output/tts-server-go ./cmd/cli
./output/tts-server-go -use-dns-edge
```

打开 http://localhost:1233 配置朗读。也可以直接运行：

```sh
go run ./cmd/cli -port 1233 -use-dns-edge
```

可选参数：`-port` 指定端口；`-token` 设置合成接口验证令牌（请求头 `Token`）；
`-use-dns-edge` 使用 DNS 解析 Edge 服务，避免使用内置旧 IP 列表。

## 项目功能

这是一个调用微软在线语音接口的 HTTP 文本转语音服务，需要联网。

- 内置网页用于选择发音人、音频格式和朗读配置，生成并导入阅读（Legado）APP 的朗读引擎。
- `/api/ra`：Edge 大声朗读，POST SSML，使用 `Format` 请求头指定音频格式。
- `/api/azure`：Azure 演示语音合成，POST SSML，支持发音人提供的风格、角色等配置。
- `/api/creation`：Speech Studio Creation 合成，POST JSON，包含文本、发音人、语速、音量、风格和音频格式。
- `/api/azure/voices`、`/api/creation/voices`：获取在线发音人数据。
- `/api/legado`：生成阅读 APP 的导入配置。

这些是旧版微软演示/消费端接口。此次本机实测：Edge 返回 403，Creation 认证返回 401，
Azure WebSocket 连接超时。Go 编译、测试通过不代表这些在线接口仍可合成语音；
恢复合成需要另行更新微软接口协议或接入正式 Azure Speech 服务。

## 测试

```sh
go test -timeout 30s ./...
go vet ./...
```

默认测试无需访问微软服务，关闭服务测试会自动完成。
在线接口测试需显式启用，且可能因上游接口变化或网络原因失败：

```sh
TTS_INTEGRATION_TEST=1 go test -timeout 60s ./tts/edge ./tts/azure ./tts/creation
```
