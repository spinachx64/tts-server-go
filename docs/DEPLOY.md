# GitHub Actions 自动构建与部署到 cook

工作流：[deploy-cook.yml](../.github/workflows/deploy-cook.yml)。
推送 `master` 的 Go 源码、依赖、网页资源、部署脚本、部署验证脚本或该工作流变更时，自动测试、构建 Linux amd64 二进制并通过 SSH 部署。
也可在 Actions 中手动运行 **Build and deploy to cook**。PR 只测试和构建，不部署。
README 等纯文档修改不会触发自动部署。构建产物保留 14 天。

目前 cook 是 Ubuntu 22.04、x86_64，并且 systemd 正常运行。服务器无需安装 Go、Python 或 edge-tts。
本地 SSH 别名 `cook` 不会自动出现在 GitHub Runner 中，必须配置它可访问的真实 IP 或域名。

## 1. 服务器准备

需要以下条件：

- Linux amd64，支持 systemd。
- 安装 OpenSSH Server、sudo、curl、tar、coreutils、util-linux（提供 flock）。
- GitHub 的 Runner 可访问 SSH 端口；默认 22。服务能访问微软接口的 HTTPS / WSS 端口 443。
- API 端口可用；默认 1233。需要远程调用时，在防火墙或反向代理中配置相应访问。
- 一次性使用 root 或 sudo 安装用户、目录、systemd unit 和限定权限的 sudoers。

Ubuntu 安装依赖：

```sh
sudo apt-get update
sudo apt-get install -y openssh-server sudo curl tar coreutils util-linux
```

将仓库克隆到服务器的临时目录，然后执行初始化脚本：

```sh
git clone https://github.com/spinachx64/tts-server-go.git
cd tts-server-go
sudo bash deploy/install-server.sh
```

如果不安装 Git，也可在本机上传 `deploy/` 目录后执行其中的脚本。
脚本可重复执行，已有 `/etc/tts-server-go.env` 不会被覆盖。
它会准备：

| 项目 | 位置 / 用途 |
|---|---|
| `tts` 用户 | 运行服务，不允许交互登录 |
| `ttsdeploy` 用户 | SSH 上传部署文件；与服务运行用户分离 |
| `/opt/tts-server-go/incoming` | 部署压缩包暂存 |
| `/opt/tts-server-go/releases` | 每次构建的版本目录 |
| `/opt/tts-server-go/current` | 当前版本软链接，首次部署时创建 |
| `/etc/tts-server-go.env` | 端口和 API Token，仅 root 可读 |
| `/etc/systemd/system/tts-server-go.service` | systemd 服务 |
| `/usr/local/libexec/tts-server-go-deploy` | 校验、切换版本及健康检查脚本 |
| `/etc/sudoers.d/tts-server-go-deploy` | 仅允许重启和检查该服务的状态，无需密码 |

服务会设置为开机启动，首次成功部署时才启动。初始化脚本不会安装二进制，也不会启动另一个项目中的 edge-tts。

## 2. 设置 API Token 和端口

初始化脚本会生成随机 Token，默认端口为 1233。用 root 查看或修改：

```sh
sudoedit /etc/tts-server-go.env
```

格式示例：

```ini
TTS_PORT=1233
TTS_TOKEN=替换为你自己的随机访问令牌
```

客户端发送 `Token: <TTS_TOKEN的值>`。这是你自己的 API 访问密码，不是 SSH 私钥，也不是微软的客户端令牌。
设置 `TTS_TOKEN=` 表示关闭 Token 验证。修改配置后需要重启服务。

如服务器需要代理，可在同一文件中增加：

```ini
HTTPS_PROXY=http://127.0.0.1:7890
NO_PROXY=127.0.0.1,localhost
```

更改 `TTS_PORT` 时，GitHub 变量 `COOK_API_PORT` 也必须同步修改，部署脚本用它进行健康检查。

需要按用户分发 Token 时，见 [SQLite Token 配置](TOKENS.md)。使用 `TTS_TOKEN_DB` 时清空 `TTS_TOKEN`，由外部程序管理数据库。

## 3. 准备独立的 SSH 部署密钥

在本机生成专用密钥，保持私钥无口令，以便 Actions 非交互运行：

```sh
ssh-keygen -t ed25519 -C github-actions-tts -f ./cook-tts-deploy -N ''
```

将公钥 `cook-tts-deploy.pub` 的整行内容添加到 cook 的 `/home/ttsdeploy/.ssh/authorized_keys`。
服务器上创建目录并设置权限：

```sh
sudo install -d -o ttsdeploy -g ttsdeploy -m 700 /home/ttsdeploy/.ssh
sudo touch /home/ttsdeploy/.ssh/authorized_keys
sudo chown ttsdeploy:ttsdeploy /home/ttsdeploy/.ssh/authorized_keys
sudo chmod 600 /home/ttsdeploy/.ssh/authorized_keys
sudoedit /home/ttsdeploy/.ssh/authorized_keys
```

只追加公钥，不覆盖已有条目。公钥条目可加 `restrict` 前缀以禁用端口转发和终端分配；部署仅使用文件上传和远程命令。

在本机确认能以 `ttsdeploy` 登录，并能执行限定的 sudo 操作：

```sh
ssh -i ./cook-tts-deploy ttsdeploy@服务器IP 'test -w /opt/tts-server-go/incoming'
```

私钥不应提交到仓库。

## 4. 获取并核对 SSH 主机密钥

从可正常登录的终端获取主机公钥指纹：

```sh
ssh cook 'ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub'
```

在本机扫描真实 IP 或域名，并确认指纹与服务器上一致：

```sh
ssh-keyscan -t ed25519 -p 22 服务器IP > cook-known-hosts
ssh-keygen -lf cook-known-hosts
```

`cook-known-hosts` 的内容用于 `COOK_KNOWN_HOSTS`。
非 22 端口时，扫描命令与 `COOK_SSH_PORT` 使用相同端口，记录中的地址为 `[主机]:端口`。
工作流启用 `StrictHostKeyChecking=yes`，不会自动信任扫描到的未知服务器。

## 5. 配置 GitHub Secrets 与 Variables

仓库 **Settings → Secrets and variables → Actions** 中设置下列值。
也可创建名为 `cook` 的 Environment 并将 Secrets 配置在其中；工作流使用该环境。

| Secrets 名称 | 值 |
|---|---|
| `COOK_HOST` | Runner 可访问的服务器 IPv4 地址或域名，例如 `cook.example.com`；不要填本机 SSH 别名 |
| `COOK_USER` | `ttsdeploy` |
| `COOK_SSH_KEY` | `cook-tts-deploy` 的完整私钥内容，包含 BEGIN / END 行 |
| `COOK_KNOWN_HOSTS` | 已核对的 `cook-known-hosts` 文件内容 |

可选 Variables：

| 变量名称 | 默认值 | 说明 |
|---|---|---|
| `COOK_SSH_PORT` | `22` | SSH 端口 |
| `COOK_API_PORT` | `1233` | 与服务器 `TTS_PORT` 一致，用于健康检查 |

这些值不会从你本机的 `~/.ssh/config` 或 SSH Agent 自动读取。
API Token 只放在服务器 `/etc/tts-server-go.env`，不需要上传到 GitHub。

使用 GitHub CLI 配置的例子：

```sh
gh secret set COOK_HOST --repo spinachx64/tts-server-go
gh secret set COOK_USER --repo spinachx64/tts-server-go --body ttsdeploy
gh secret set COOK_SSH_KEY --repo spinachx64/tts-server-go < cook-tts-deploy
gh secret set COOK_KNOWN_HOSTS --repo spinachx64/tts-server-go < cook-known-hosts
```

`COOK_HOST` 命令会提示输入值。仅在需要修改默认端口时设置 Variables。

## 6. 启动第一次部署

准备完成后，在 Actions 页手动运行 **Build and deploy to cook**，分支选择 `master`。

```sh
gh workflow run deploy-cook.yml --repo spinachx64/tts-server-go --ref master
```

工作流会：

1. 执行 Go 测试、vet 和 Edge / server 的竞态检查。
2. 编译不依赖 CGO 的 Linux amd64 二进制，生成 SHA-256 校验文件，并上传构建产物。
3. 使用专用 SSH 私钥和核对后的主机密钥上传压缩包。
4. 在服务器检查 SHA-256，将 `current` 原子切换到新版本，并重启 systemd 服务。
5. 检查服务状态与本机 HTTP 首页；失败时恢复上一个版本并再次检查。

四个 Secrets 全部缺失时，构建仍执行，部署会明确显示为跳过；只配了一部分时部署失败并提示补齐。
首次部署没有旧版本，因此无法回滚。健康检查确认 HTTP 服务正常，不调用收费或限流的上游合成接口。
旧版本保留在 `releases` 目录，清理时不要删除 `current` 指向的目录。

## 7. 查看运行状态与测试 API

```sh
ssh cook 'systemctl status tts-server-go.service --no-pager'
ssh cook 'journalctl -u tts-server-go.service -n 100 --no-pager'
```

服务启动后，可从可访问服务器的机器运行：

```sh
python3 scripts/test_apis.py --url http://服务器IP:1233 --token 你的API令牌
```

Creation 当前上游认证失败，因此全渠道脚本可能返回 1；请查看 Edge 和 Azure 的各项结果。

## CI 触发范围

日常 push 只由 `deploy-cook.yml` 执行测试、前端检查、Linux amd64 构建与部署。原项目的多平台构建 `test.yml` 和发布 `main.yml` 改为 `workflow_dispatch` 手动触发，不再随 push 或 CHANGELOG 变更自动启动。手动多平台任务去掉不受支持的 Windows ARM32 组合，保留 Windows ARM64；单个组合失败不会取消其它组合。
