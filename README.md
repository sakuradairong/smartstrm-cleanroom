<div align="center">

# SmartStrm Clean-room

用 Go 从零实现的 STRM 生成、任务自动化与安全播放服务。

[![CI](https://github.com/sakuradairong/smartstrm-cleanroom/actions/workflows/ci.yml/badge.svg)](https://github.com/sakuradairong/smartstrm-cleanroom/actions/workflows/ci.yml)
[![Go 1.23](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go)](go.mod)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)](docker-compose.yml)

[快速开始](#快速开始) · [功能状态](docs/FEATURE_MATRIX.md) · [配置示例](config.example.json) · [参与贡献](CONTRIBUTING.md) · [安全策略](SECURITY.md)

</div>

> [!IMPORTANT]
> **这是独立的 clean-room 实现。** 本仓库依据公开功能说明和公开协议从零编写，不包含原闭源 SmartStrm 的私有源码，不绕过许可证，也不调用未经授权的私有 API。项目名称仅用于说明兼容目标；本项目不代表原项目作者或其商业版本。

## 项目状态

当前版本已经具备可部署的核心链路：读取存储、生成和维护 STRM、调度任务、接收 Webhook、通过签名地址播放，以及使用管理页面执行常用操作。

并非所有原版公开功能都已完成。原生国内云盘、特定媒体服务器深度改写和真实第三方服务兼容性需要官方文档、测试账号或固定版本实验环境；未经验证的能力不会标记为完成。

- 逐项实现与验证结果：[`docs/FEATURE_MATRIX.md`](docs/FEATURE_MATRIX.md)
- 外部验证条件与阻塞项：[`docs/EXTERNAL_VALIDATION.md`](docs/EXTERNAL_VALIDATION.md)
- 上游公开版本差异参考：[`docs/UPSTREAM_RELEASE_NOTES.md`](docs/UPSTREAM_RELEASE_NOTES.md)

## 主要能力

### 存储与文件操作

- Local、WebDAV、OpenList、123 云盘官方开发者 API，以及只读 ANi Open RSS/XML 驱动
- `123_open` 使用 123 云盘官方开放平台；`quark`、`115`、`189`、`123` 仍是 OpenList 适配器别名，已有配置不会自动切换协议
- 存储浏览、筛选、自然排序、新建目录、重命名、移动和删除
- OpenList 显式分页、WebDAV 规范化 PROPFIND、元数据响应硬上限和严格 JSON/XML 解码
- 本地根目录和符号链接边界保护；远端错误、直链和响应头脱敏

### STRM 与任务自动化

- 递归生成、增量运行、附属文件复制和可选同步删除
- 媒体扩展名、大小范围、保留规则和任务级插件
- 标准五段 Crontab、本地目录监听、串行队列、停止与重新运行
- 单条条目失败时继续处理兄弟条目；部分失败会跳过同步删除
- 真实输出预览，不写文件、不修改远端

### Webhook 与媒体联动

- 通用任务 Webhook、QAS/CloudSaver `a_task`、`cs_strm` 和 `web_save`
- Emby 删除事件同步删除源文件
- CloudDrive2 与 MoviePilot 的公开负载映射基础
- HMAC 签名播放地址，Local/WebDAV Range 流与 OpenList 302
- Emby/Jellyfin `PlaybackInfo` 公开协议改写基础；真实客户端兼容状态以功能矩阵为准

### 管理与命名

- Basic Auth 管理 UI 和 API
- 任务状态、实时历史 SSE、配置保存/恢复与敏感字段脱敏
- 正则、顺序、魔法变量和 TMDB movie/TV 批量命名
- TMDB 海报代理、剧集标题、`{title_original}` 和空原始标题回退
- ffmpeg 视频封面提取、预览和超时控制

## 快速开始

### Docker Compose（推荐）

要求：Docker Engine 和 Docker Compose v2。

```bash
git clone https://github.com/sakuradairong/smartstrm-cleanroom.git
cd smartstrm-cleanroom

mkdir -p runtime/config runtime/media runtime/strm
cp config.example.json runtime/config/config.json

# 只需修改 public_url、管理员口令和 Webhook Token。
${EDITOR:-vi} runtime/config/config.json

# 容器使用 UID/GID 10001，需要能够原子更新配置和历史文件。
chmod 600 runtime/config/config.json
sudo chown -R 10001:10001 runtime/config runtime/media runtime/strm

docker compose up -d --build
docker compose ps
```

管理页面默认位于 `http://localhost:8024`。首次登录后进入“配置”页添加存储和生成任务，验证保存后执行 `docker compose restart smartstrm`。`8097` 是可选媒体代理端口；只有通过 WebUI 配置并重启后才会监听。

健康检查：

```bash
curl --fail http://localhost:8024/health
```

停止服务：

```bash
docker compose down
```

### 直接运行

要求：Go 1.23；使用封面提取功能时还需 `ffmpeg`。

```bash
cp config.example.json config.json
chmod 600 config.json
go run ./cmd/smartstrm -config config.json
```

## 配置

[`config.example.json`](config.example.json) 只保留启动所需的基础字段。首次启动后，在管理 UI 的“配置”页添加存储、生成任务及其他业务能力；保存会严格校验整份候选配置并原子写回同一个 `0600` 文件。存储、任务、TMDB、历史和媒体代理设置支持热更新；管理监听地址、认证以及运行中的代理监听端口变更仍需重启。有运行中、排队、延迟任务或任务工具操作时，保存会被拒绝并保留原配置，待任务结束后重试；外部文件编辑则会持续重试应用。未知字段、错误类型、非法 URL、危险目录、无效 Cron 或不支持的能力都会阻止应用。

每个任务的输出目录必须独占：不同任务不能使用相同或互为祖先的输出目录，输出目录也不能与任何 Local 存储根目录重叠。校验会解析已有祖先目录中的符号链接；实际生成、复制、清理和本地改名使用 Linux 目录句柄操作，拒绝符号链接父目录及同名改名覆盖。这些安全文件操作目前要求 Linux；其他系统会返回不支持错误，推荐通过 Linux Docker 运行。

环境变量覆盖会在每次激活配置时重新应用，环境中的密钥不会写入配置或备份。设置 `WEBHOOK_TOKEN` 时，管理页重置 Token 会返回冲突；应修改部署环境并重启。

| 区域 | 用途 | 注意事项 |
| --- | --- | --- |
| `admin` | 管理 UI/API 的 Basic Auth | 必须更换示例密码 |
| `webhook_token` | Webhook 鉴权和签名密钥 | 使用强随机值；更换后需重新生成 STRM |
| WebUI `存储` | Local/WebDAV/OpenList/123 官方 API/ANi Open | 凭据经认证 API 写入 `0600` 配置；123 官方驱动的下载与播放需要对应开放平台权益和直链空间 |
| WebUI `生成任务` | 源路径、目标、调度和插件 | `sync_delete` 有破坏性，首次运行保持关闭 |
| WebUI `高级能力` | 历史、TMDB、媒体工具、媒体代理、联动和全局插件 | 复杂映射/插件只编辑对应 JSON 区块，不暴露整份原始配置 |

基础启动字段（`listen`、`public_url`、`admin`、`webhook_token`）不能通过业务设置 API 修改，避免错误操作导致管理入口失联。存储口令、Token 和 TMDB Key 在 WebUI 中显示为 `********`；保持该值或留空会保留已有密钥。完整持久化结构仍可通过认证后的 `GET /api/config` 查看脱敏结果，供备份和自动化使用。

### 123 云盘官方 API

1. 前往 [123 云盘开放平台](https://www.123pan.com/developer) 创建开发者应用，取得 Client ID 和 Client Secret。
2. 在 123 云盘中为计划挂载的子目录启用“直链空间”，并从目录 URL 获取 File ID。文件列表等基础能力可用时，不代表账号已具备下载/播放权益；直链通常需要相应 VIP/开放平台权限。
3. 在 WebUI 的“配置 → 存储 → 添加存储”选择“123 云盘官方 API”，填写上述凭据和挂载目录 File ID；根目录 ID 为 `0`，但根目录本身不能提供直链。
4. 保存后按页面提示重启服务。若任务要使用稳定 File ID STRM，启用任务的“文件 ID 模式”。

原生驱动类型为 `123_open`。旧类型 `123` 继续表示 OpenList 适配器，不会在升级时静默改变认证方式。Client Secret 只在 `0600` 配置中持久化，管理 API 返回 `********`；编辑时保留该值或留空均不会清除真实密钥。

建议生成随机 Token：

```bash
openssl rand -hex 32
```

### 最小任务示例

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

## 常用 API

以下示例不会把凭据写进 URL：

```bash
export SMARTSTRM_URL=http://localhost:8024
export SMARTSTRM_ADMIN='admin:your-password'
export SMARTSTRM_TOKEN='your-webhook-token'

# 查看任务
curl --user "$SMARTSTRM_ADMIN" "$SMARTSTRM_URL/api/tasks"

# 运行任务的一个源目录
curl --user "$SMARTSTRM_ADMIN" \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"path":"/movies/2025"}' \
  "$SMARTSTRM_URL/api/tasks/movies/run"

# 外部工具触发任务
curl --request POST \
  --header "Authorization: Bearer $SMARTSTRM_TOKEN" \
  --header 'Content-Type: application/json' \
  --data '{"task_id":"movies","path":"/movies/2025"}' \
  "$SMARTSTRM_URL/webhook/run"
```

Emby Webhook 地址：

```text
https://your-smartstrm.example/webhook/emby?token=YOUR_TOKEN
```

将含 Token 的 URL 视为秘密，不要提交到 Issue、日志或截图。

## 安全提示

> [!CAUTION]
> `sync_delete` 会删除目标目录中源端已不存在的 `.strm` 和复制文件；Emby 删除同步会删除真实源文件。请先使用独立测试目录、关闭同步删除并检查预览。

工具箱的“清除已验证的 STRM”只删除能用当前密钥验证、同源且源路径属于该任务的 STRM，按文件计数。原始媒体、字幕、海报、旧密钥签名和归属不明的文件会保留；无法解析的稳定 File ID 也会保留。这是保守清理策略，尚未实现完整输出清单；`sync_delete` 仍按扩展名和当前扫描结果清理独占输出目录。

清理执行会先将候选条目原子隔离到其父目录下权限为 `0700` 的 `.smartstrm-clear-*` 目录，再核对内容与已验证快照一致后删除。内容变化时无覆盖恢复原名；若原名已被新文件占用，则保留隔离副本并返回恢复位置。后续清理及同步删除会跳过这些恢复目录。该机制保护文件名替换竞态，前提是隔离目录由本操作独占，捕获的文件不经已有文件句柄或硬链接并发改写。

- 不要直接将管理端口暴露到互联网；应放在可信网络或受控反向代理之后。
- 配置、备份和历史包含口令、Token、路径或媒体信息，必须保持 `0600`。
- Local root 必须是绝对、非根目录；容器挂载应遵循最小权限原则。
- 不要在仓库中提交 `config.json`、`runtime/`、真实 Webhook 负载或第三方凭据。
- 安全问题请按 [`SECURITY.md`](SECURITY.md) 私下报告，不要创建公开 Issue。

## 开发与验证

```bash
find cmd internal -type f -name '*.go' -print0 | xargs -0 gofmt -w
go mod tidy
go vet ./...
go test -race ./...
cd web && npm ci
npm run typecheck
npm run test:unit
npm run build
cd ..
python3 -m json.tool config.example.json >/dev/null
docker compose config -q
git diff --check
```

前端要求 Node.js 22。Vite 产物由 `go:embed` 嵌入 Go 二进制，`internal/app/ui/dist/` 是本地生成目录，不提交到 Git。

构建与冒烟：

```bash
docker compose build smartstrm
docker compose up -d smartstrm
curl --fail http://localhost:8024/health
```

目录结构：

```text
cmd/smartstrm/       程序入口
internal/app/        HTTP API、管理 UI 和集成入口
internal/config/     严格配置、迁移和敏感字段处理
internal/storage/    存储驱动与安全路径/协议实现
internal/task/       调度、监听、生成、插件和任务工具
internal/tmdb/       TMDB 客户端与受限图片代理
internal/mediaproxy/ 媒体服务器代理与 PlaybackInfo 改写
docs/                功能矩阵和外部验证要求
```

提交代码前请阅读 [`CONTRIBUTING.md`](CONTRIBUTING.md)。项目采用 Conventional Commits、Go 格式化、竞态测试和最小可复现实验服务。

## 已知边界

- 夸克、115、天翼和迅雷仍需通过 OpenList；123 已提供官方开发者 API 驱动，但真实下载/播放取决于账号权益和目录直链设置。
- 当前驱动不伪造稳定 File ID；不具备可信 File ID API 时使用路径模式。
- Plex、飞牛影视及特定 Emby/Jellyfin 客户端的深度兼容尚未获得完整真实环境验证。
- TMDB mock 和浏览器流程已验证，但实网限流、代理和地区响应仍需要真实 API Key。
- 本项目不会逆向闭源程序、绕过商业许可或猜测私有协议字段。

## 参与和支持

- Bug 与可复现问题：使用 [Bug report](.github/ISSUE_TEMPLATE/bug_report.yml)
- 功能建议：使用 [Feature request](.github/ISSUE_TEMPLATE/feature_request.yml)
- 贡献代码：阅读 [`CONTRIBUTING.md`](CONTRIBUTING.md)
- 社区互动：遵守 [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)
- 使用支持：参阅 [`SUPPORT.md`](SUPPORT.md)
- 安全漏洞：阅读 [`SECURITY.md`](SECURITY.md)

## 许可证与归属

本 clean-room 实现以 [GNU Affero General Public License v3.0](LICENSE) 发布。通过网络向用户提供修改后的版本时，也需要遵守 AGPL-3.0 的源码提供义务。

Copyright © 2025 SmartStrm Clean-room contributors.

第三方组件及容器软件包说明见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)。

原 SmartStrm 的名称、图标、截图、公开文档和历史版本说明归其各自权利人所有。本项目仅引用必要的公开版本说明来记录 clean-room 兼容目标，不再分发原项目图标、截图或闭源安装/更新脚本；这不代表官方认可或合作关系。第三方商标归其所有者所有。
