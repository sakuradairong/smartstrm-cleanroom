<div align="center">

# SmartStrm Clean-room

把网盘和本地目录里的媒体，变成 Emby、Jellyfin 能扫描、能播放的 STRM 库。

[![CI](https://github.com/sakuradairong/smartstrm-cleanroom/actions/workflows/ci.yml/badge.svg)](https://github.com/sakuradairong/smartstrm-cleanroom/actions/workflows/ci.yml)
[![Go 1.23](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go)](go.mod)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)](docker-compose.yml)

[它做什么](#它做什么) · [快速开始](#快速开始) · [现在做到哪一步](#现在做到哪一步) · [配置](#配置) · [安全](#安全)

</div>

> [!IMPORTANT]
> **这是独立的 clean-room 实现。** 本仓库依据公开功能说明和公开协议从零编写，不包含原闭源 SmartStrm 的私有源码，不绕过许可证，也不调用未经授权的私有 API。项目名称只说明兼容目标。本项目不代表原项目作者或其商业版本。

## 它做什么

媒体服务器认的是本地文件。网盘里的片子通常不能直接放进媒体库。SmartStrm 站在中间：扫描一份存储，为每个媒体文件写一条 `.strm`，里面是带签名的播放地址。Emby 或 Jellyfin 扫描这些 STRM；播放时请求回到本服务，再转到 Local、WebDAV 或 OpenList 上的原文件。

一次完整使用是这条链路：

```text
存储里的媒体
    │  任务扫描、过滤、写出 .strm
    ▼
本地 STRM 目录  ←  Emby / Jellyfin 当普通媒体库扫描
    │  播放
    ▼
签名地址回到 SmartStrm，再转到源文件
```

管理界面有五个页面：仪表盘看运行概况，任务里运行、停止、预览和清理，存储浏览里管理文件，运行日志看历史和实时事件，配置里添加存储和任务。服务是一个 Go 进程，前端构建后嵌进二进制。

生成、复制附属文件、同步清理、内容替换，以及本地改名和移动，使用 Linux 目录句柄。其他系统会返回不支持。请用本仓库的 Linux 容器跑这些操作。`go run` 在 Windows 或 macOS 上可以打开管理页，不能当作生成和改名的运行环境。

## 快速开始

要求 Docker Engine 和 Docker Compose v2。

```bash
git clone https://github.com/sakuradairong/smartstrm-cleanroom.git
cd smartstrm-cleanroom

mkdir -p runtime/config runtime/media runtime/strm
cp config.example.json runtime/config/config.json
```

编辑 `runtime/config/config.json` 里这几项：

| 字段 | 填什么 |
| --- | --- |
| `public_url` | 媒体服务器访问得到的地址，例如 `http://192.168.1.10:8024`。STRM 里的播放地址用它拼出来 |
| `admin.username` / `admin.password` | 管理页的 Basic Auth。用户名为空时管理接口不校验身份 |
| `webhook_token` | 长随机值，同时是 Webhook 鉴权密钥和播放签名密钥 |

```bash
openssl rand -hex 32

chmod 600 runtime/config/config.json
# 容器进程是 UID 10001（Compose 写为 10001:0）。目录要让这个用户可写。
sudo chown -R 10001 runtime/config runtime/media runtime/strm

docker compose up -d --build
curl --fail http://localhost:8024/health
```

打开 `http://localhost:8024`，在「配置」里添加存储和生成任务，然后点「应用所有修改」。存储、任务、TMDB、历史，以及媒体代理的启用、上游、服务器类型和 PlaybackInfo 改写会立刻生效。管理监听地址 `listen`、管理员账号，以及媒体代理已经在听时再改端口，要重启进程后才切换。

Compose 已经把宿主机的 `8024` 和 `8097` 映射进容器。第一次启用代理并把监听填成 `:8097`，保存后就会开始监听；代理没启用时，`8097` 上没有进程。健康检查只覆盖 `8024`。

有任务正在运行、排队、延迟，或任务工具还在操作时，页面保存会被拒绝，原配置保持有效。等这些操作结束后再保存。直接改配置文件则会等到可以应用时自动重试。

```bash
docker compose down
```

不使用容器时，在 Linux 上：

```bash
cp config.example.json config.json
chmod 600 config.json
go run ./cmd/smartstrm -config config.json
```

配置路径也可以用环境变量 `SMARTSTRM_CONFIG`。封面提取需要系统里有 `ffmpeg`；容器镜像已经带上。

## 配置

[`config.example.json`](config.example.json) 只够把服务启动起来。存储、任务和高级能力在管理页里添加，保存时整份配置会严格校验，并以 `0600` 原子写回同一个文件。未知字段、错误类型、非法 URL、危险目录、无效 Cron 或不支持的驱动都会阻止应用。

`listen`、`public_url`、`admin`、`webhook_token` 不在业务设置里改，避免一次保存把管理入口改丢。存储口令、Token 和 TMDB Key 在页面上显示为 `********`；保留这个占位或留空，都会继续使用已有密钥。

环境变量 `PORT`、`ADMIN_USERNAME`、`ADMIN_PASSWORD`、`WEBHOOK_TOKEN` 在每次激活配置时覆盖对应字段，并且不会写进配置文件或备份。设置了 `WEBHOOK_TOKEN` 时，页面上的「重置 Token」会返回冲突，应改部署环境后重启。更换签名密钥后，已生成的 STRM 需要重新生成。

每个任务的输出目录必须独占：不能与另一个任务的输出目录相同或互为祖先，也不能和任何 Local 存储根重叠。

### 添加第一条任务

在「配置 → 生成任务」里填写，或让自动化写入同等结构：

```json
{
  "id": "movies",
  "name": "电影",
  "storage_id": "local-media",
  "source": "/movies",
  "destination": "/strm/movies",
  "schedule": "0 */6 * * *",
  "incremental": true,
  "sync_delete": false,
  "media_ext": [".mkv", ".mp4", ".iso"],
  "copy_ext": [".jpg", ".png", ".nfo", ".srt"]
}
```

`schedule` 是标准五段 Cron。本地存储还可以打开目录监听。增量运行只处理有变化的条目。单条失败会继续处理其余条目，并跳过这一轮同步删除。运行前可以预览将要写出的 STRM，预览不写文件、不改远端。

`sync_delete` 首次保持关闭。它会删除输出目录里源端已经不存在的 `.strm` 和复制文件。

### 存储怎么选

| 类型 | 什么时候用 |
| --- | --- |
| `local` | 已经挂进本机或容器的目录。根必须是绝对路径，且不能是文件系统根 |
| `webdav` | 标准 WebDAV。播放走 Range 代理 |
| `openlist` | OpenList。播放返回经过校验的 302。`quark`、`115`、`189`、`123` 是这个驱动的别名 |
| `123_open` | 123 云盘开放平台。和上面的 `123` 别名是两套凭据，升级不会自动改协议 |
| `ani_open` | ANi Open 的只读 RSS/XML。不能改、不能删 |

123 官方驱动：

1. 在 [123 云盘开放平台](https://www.123pan.com/developer) 创建应用，取得 Client ID 和 Client Secret。
2. 给要挂载的子目录打开直链空间，从目录 URL 取得 File ID。能列出文件，不表示账号已经有下载和播放权益。
3. 在「配置 → 存储」选择「123 云盘官方 API」，填入凭据和挂载目录 File ID。根目录 ID 是 `0`，根目录本身不提供直链。
4. 需要稳定 File ID STRM 时，打开任务的文件 ID 模式。

Client Secret 只留在 `0600` 配置里。真实账号的下载、播放和限频仍待外部验证，见 [外部验证清单](docs/EXTERNAL_VALIDATION.md)。

## 播放和外部触发

管理 API 使用 Basic Auth。Webhook 接受查询参数 `token`、路径中的 token，或 `Authorization: Bearer`。带 token 的 URL 按秘密保管，不要放进 Issue、日志或截图。

```bash
export SMARTSTRM_URL=http://localhost:8024
export SMARTSTRM_ADMIN='admin:your-password'
export SMARTSTRM_TOKEN='your-webhook-token'

curl --user "$SMARTSTRM_ADMIN" "$SMARTSTRM_URL/api/tasks"

curl --user "$SMARTSTRM_ADMIN" \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"path":"/movies/2025"}' \
  "$SMARTSTRM_URL/api/tasks/movies/run"

curl --request POST \
  --header "Authorization: Bearer $SMARTSTRM_TOKEN" \
  --header 'Content-Type: application/json' \
  --data '{"task_id":"movies","path":"/movies/2025"}' \
  "$SMARTSTRM_URL/webhook/run"
```

| 入口 | 用途 |
| --- | --- |
| `POST /webhook/run` | 通用任务触发，含公开的 `a_task` 负载 |
| `POST /webhook/emby?token=…` | Emby 删除事件。确认后会删除对应源文件 |
| `POST /webhook/{token}` | QAS / CloudSaver，以及 `event=web_save` 的网页转存 |
| `POST /webhook/{token}/file_notify` | CloudDrive2 `file_notify` |
| `POST /webhook/{token}/moviepilot` | MoviePilot `transfer.complete` |

媒体代理监听独立端口，把固定上游的 Emby 或 Jellyfin 流量转过来，并可按公开协议改写 `PlaybackInfo` 里的同源签名 STRM。真实客户端的兼容范围以功能矩阵为准。

## 现在做到哪一步

逐项证据在 [`docs/FEATURE_MATRIX.md`](docs/FEATURE_MATRIX.md)。没有真实协议或可复现实验的能力不会写成完成。

| 状态 | 范围 |
| --- | --- |
| 可以部署 | Local、WebDAV、OpenList、ANi Open；递归和增量生成；Cron 与本地监听；串行队列、停止和预览；管理界面；HMAC 播放；Local/WebDAV Range 与 OpenList 302；上表中的 Webhook |
| 实现了，还差真实环境 | 123 官方 API 的权益账号下载与播放；TMDB 的实网 Key、限流和地区差异；Emby/Jellyfin 真实客户端上的 PlaybackInfo 改写 |
| 还没有 | 夸克、115、天翼、迅雷的原生驱动（前三个可走 OpenList）；Plex 与飞牛的服务器改写；飞牛刷新；首播优化、外部播放器和隐藏推广 |

夸克等网盘在官方文档、测试账号和授权协议齐备之前，继续经 OpenList 使用。本项目不伪造稳定 File ID，也不猜测私有协议字段。

## 安全

> [!CAUTION]
> `sync_delete` 会删除目标目录中源端已不存在的 `.strm` 和复制文件。Emby 删除同步会删除真实源文件。先用独立测试目录，保持同步删除关闭，并看过预览。

工具箱里的「清除已验证的 STRM」只删除能用当前密钥验证、且源路径属于该任务的 STRM。原始媒体、字幕、海报、旧密钥签名和归属不明的文件会保留。清理先把候选文件隔离到父目录下权限为 `0700` 的 `.smartstrm-clear-*`，核对内容后再删除；内容已变或原名被占用时，会留下恢复副本。

- 管理端口放在可信网络或受控反向代理后面。
- 配置、备份和历史含有口令、Token 或媒体路径，保持 `0600`。历史默认写在配置文件同目录的 `history.jsonl`。
- Local 根必须是绝对路径，且不能是文件系统根。容器卷按最小权限挂载。
- 不要提交 `config.json`、`runtime/`、真实 Webhook 负载或第三方凭据。
- 漏洞按 [`SECURITY.md`](SECURITY.md) 私下报告。

## 开发

要求 Go 1.23、Node.js 22。提交前的完整清单在 [`CONTRIBUTING.md`](CONTRIBUTING.md)。

```bash
make check

cd web && npm ci
npm run typecheck && npm run test:unit && npm run build
```

`make check` 会跑 vet、竞态测试、Markdown 链接、页面样式、示例配置和 Compose 校验。`npm run build` 把 `go:embed` 需要的产物写到 `internal/app/ui/dist/`。这个目录是本地生成的，不提交。

```text
cmd/smartstrm/       程序入口
internal/app/        HTTP API、管理 UI、Webhook 和集成入口
internal/config/     严格配置、迁移和敏感字段
internal/storage/    存储驱动
internal/task/       调度、监听、生成和任务工具
internal/tmdb/       TMDB 客户端与受限图片代理
internal/mediaproxy/ 媒体服务器代理与 PlaybackInfo 改写
docs/                功能矩阵和外部验证要求
```

## 参与

- 缺陷：[Bug report](.github/ISSUE_TEMPLATE/bug_report.yml)
- 功能建议：[Feature request](.github/ISSUE_TEMPLATE/feature_request.yml)
- 贡献代码：[`CONTRIBUTING.md`](CONTRIBUTING.md)
- 社区规范：[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)
- 使用问题：[`SUPPORT.md`](SUPPORT.md)
- 安全漏洞：[`SECURITY.md`](SECURITY.md)

## 许可证

本实现以 [GNU Affero General Public License v3.0](LICENSE) 发布。通过网络向用户提供修改后的版本时，需要遵守 AGPL-3.0 的源码提供义务。

Copyright © 2025 SmartStrm Clean-room contributors.

第三方组件及容器软件包见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)。

原 SmartStrm 的名称、图标、截图、公开文档和历史版本说明归其各自权利人所有。本项目只引用必要的公开版本说明来记录兼容目标，不再分发原项目图标、截图或闭源安装、更新脚本。第三方商标归其所有者所有。
