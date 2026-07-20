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

- Local、WebDAV、OpenList，以及只读 ANi Open RSS/XML 驱动
- 国内云盘优先通过 OpenList 聚合接入；`quark`、`115`、`189`、`123` 是 OpenList 适配器别名，不是原生驱动
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

# 修改 public_url、管理员口令、Webhook Token、存储和任务。
# 首次部署建议保持 sync_delete=false。
${EDITOR:-vi} runtime/config/config.json

# 容器使用 UID/GID 10001，需要能够原子更新配置和历史文件。
chmod 600 runtime/config/config.json
sudo chown -R 10001:10001 runtime/config runtime/media runtime/strm

docker compose up -d --build
docker compose ps
```

管理页面默认位于 `http://localhost:8024`。`8097` 是可选媒体代理端口；只有配置中启用 `media_proxy` 后才会监听。

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

完整字段见 [`config.example.json`](config.example.json)。配置使用严格 JSON 解码：未知字段、错误类型、非法 URL、危险本地根目录、无效 Cron 或不支持的能力会阻止启动或保存，不会只启动部分后台任务。

| 区域 | 用途 | 注意事项 |
| --- | --- | --- |
| `admin` | 管理 UI/API 的 Basic Auth | 必须更换示例密码 |
| `webhook_token` | Webhook 鉴权和签名密钥 | 使用强随机值；更换后需重新生成 STRM |
| `storages` | Local/WebDAV/OpenList/ANi Open | 凭据只写入 `0600` 配置文件；国内云盘凭据建议保存在 OpenList |
| `tasks` | 源路径、目标、调度和插件 | `sync_delete` 有破坏性，首次运行保持关闭 |
| `history` | JSONL 运行历史 | 文件按敏感数据使用 `0600` 权限 |
| `tmdb` | 搜索、详情和海报 | 真实服务需要用户自己的 API Key |
| `media_proxy` | 固定上游媒体代理 | 默认关闭；上游必须是无凭据/Query/Fragment 的 HTTP(S) URL |

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
python3 -m json.tool config.example.json >/dev/null
docker compose config -q
git diff --check
```

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

- 当前不提供夸克、115、天翼、123、迅雷的原生账号驱动；请使用 OpenList。
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
