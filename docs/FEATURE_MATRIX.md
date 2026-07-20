# SmartStrm clean-room 功能契约矩阵

本矩阵基于公开文档（`smartstrm.github.io`）和当前仓库实现维护。`✅` 表示已有自动化或运行证据，`🟡` 表示部分实现，`⬜` 表示尚未实现。功能只有在真实协议或可复现实验服务验证后才能标记为 `✅`。

## 存储与文件管理

| 功能 | 状态 | 当前证据 / 剩余工作 |
|---|---:|---|
| 本地存储 | ✅ | `internal/storage/local.go`；生命周期、边界与竞态测试；root 在保存/启动前必须绝对且非文件系统根，解析符号链接后重复校验；隐藏/拒读逃逸链接，Delete/Rename/Move 操作链接本身而不跟随目标，单元及容器外部目标完整性验证覆盖 |
| WebDAV | ✅ | PROPFIND/DELETE/MKCOL/MOVE/Range 代理实验服务器测试；不依赖响应顺序，仅按规范化 href 跳过当前目录，兼容绝对 URL/路径、相对与 percent-encoded href、HTTP-date、RFC3339 UTC 和空修改时间 |
| OpenList | ✅ | list/get/remove/mkdir/rename/move API 实验服务器测试；显式 200 条分页完整读取 450 条目录，支持 `total` 收敛、只在第一页强刷，并拒绝重复页、提前空页和超过 100,000 条的异常响应 |
| 存储元数据响应上限 | ✅ | ANi Open XML、OpenList JSON 与 WebDAV PROPFIND XML 均使用 16 MiB 硬上限并拒绝超限、第二根对象或尾随字符；媒体直链和 Range 代理保持流式，边界/未知长度/协议竞态与容器测试覆盖 |
| 存储 endpoint 安全校验 | ✅ | 保存和启动前统一要求 HTTP(S) 且禁止内嵌凭据/Fragment；OpenList/WebDAV 基地址还禁止 Query 并保留合法子路径，ANi feed 允许 Query；未知类型拒绝，单元/竞态/容器无副作用启动失败覆盖 |
| 远程直链安全校验 | ✅ | OpenList `raw_url`、ANi feed link 与统一 `/stream` 重定向出口三层验证绝对 HTTP(S)、禁止内嵌凭据/Fragment、不回显恶意 URL，同时保留 CDN 签名 Query；协议 mock、竞态和容器 302/502 覆盖 |
| 远端错误脱敏 | ✅ | WebDAV/OpenList HTTP 错误不回显 reason phrase/正文，OpenList 业务错误不回显 `message`，仅保留协议和数字 status/code；恶意凭据、token、换行内容的单元/竞态/容器 API 无泄露验证覆盖 |
| WebDAV 播放响应防护 | ✅ | 仅代理媒体 200/206 与无正文 304/416，其他状态映射脱敏 502；416 只保留安全 Range 元数据，HEAD 上游使用 HEAD 且不复制正文，响应头允许列表拒绝 `Set-Cookie`；竞态及容器签名播放覆盖 |
| 夸克、夸克 TV | 🟡 | 当前可经 OpenList 使用；原生合规适配器和真实账号验证待完成 |
| 115 开放平台 | 🟡 | 当前可经 OpenList 使用；官方开放 API 适配与真实账号验证待完成 |
| 天翼云盘与 `.cas` | 🟡 | 当前可经 OpenList 使用；官方 API、家庭云和 CAS 秒传验证待完成 |
| 123 云盘开放平台 | 🟡 | 当前可经 OpenList 使用；开发者 API 与权益账号验证待完成 |
| 迅雷网盘 | ⬜ | OAuth、容量、列表、重命名、直链模式待实现和验证 |
| ANi Open | ✅ | 官方 `ani-download.xml`、季度虚拟目录、大小/时间、直链、缓存、只读约束、模拟协议与实网 feed 验证 |
| 存储浏览、导航、筛选 | ✅ | `/api/storages/*/entries` 与内嵌管理 UI；自然升/降序、名称列宽持久化、目录/视频/图片/音频/字幕图标、实时筛选数/目录总数及 `aria-live` 状态通过竞态、容器和真实 Chrome 交互验证 |
| WebUI 配置管理 | ✅ | 基础文件只保留监听、公开 URL、管理员与 Webhook Token；受限 `/api/config/managed` 结构化管理存储、任务和高级能力，严格拒绝基础字段覆盖，密钥脱敏/保留、原子 `0600` 持久化、备份、重启提示、竞态、隔离 Docker 和真实 Chrome CRUD 已验证 |
| 新建目录、重命名、移动、删除 | ✅ | Local/WebDAV/OpenList 统一接口与驱动测试 |
| 复制签名 STRM 地址 | ✅ | `/api/storages/{id}/stream-url` 与 API 测试；文件行显式按钮及鼠标右键/`Shift+F10` 无障碍菜单复用签名接口，目录不显示复制项，并有竞态、容器和 Chrome 验证 |
| 正则、顺序、魔法变量批量命名 | ✅ | `internal/rename`；预览、冲突检查、两阶段事务及 UI |

## STRM 与任务

| 功能 | 状态 | 当前证据 / 剩余工作 |
|---|---:|---|
| 递归生成、增量生成、同步删除 | ✅ | `internal/task/generator.go` 与生成集成测试 |
| 单条失败隔离 | ✅ | 单条改名、复制、子目录列表、STRM 内容或写入失败会继续处理兄弟条目，最终有界聚合并计入 `failed`；任何部分失败都会禁止同步删除，避免误删，且有竞态、容器和历史验证 |
| 后缀、大小上下限、附属文件复制 | ✅ | 任务级配置和生成测试；同 stem 及 `.`/`-`/`_` 后缀元数据自动插入匹配媒体扩展名并保留大小写，已命名文件不重复插入，多媒体歧义不猜测，复制目标碰撞在写入前拒绝 |
| 标准五段 Crontab | ✅ | 共享解析器覆盖通配符、范围、列表、步长与日/周 OR 语义；配置保存前拒绝非法表达式，Manager 在启动任何 worker/watcher 前完成全量预检；`2 月 31 日` 等不可达日期通过跨闰年有界匹配、容器启动和 Chrome 无白屏验证 |
| 本地实时监听 | ✅ | 轮询监听及变更测试；原生事件后端可继续优化 |
| 任务串行队列、防并发 | ✅ | 每任务有界队列与竞态测试 |
| 任务状态低开销轮询 | ✅ | 移除重复固定心跳，改为不可重入的单一 `setTimeout` 循环；页面隐藏暂停、恢复可见立即刷新，并有渲染脚本、竞态、容器和 Chrome 请求计数验证 |
| 停止运行中任务 | ✅ | 运行级 context、管理 API/UI 与队列隔离竞态测试 |
| 目录时间检查 | ✅ | 远端目录时间与本地生成目录时间比较、无时间自动回退及 Webhook 覆盖测试 |
| 工具箱：内容替换、全量覆写、清除 | ✅ | 非根/非符号链接约束、内容替换预览、运行互斥、API/UI 与测试 |
| STRM URL 生成预览 | ✅ | 复用实际插件、目标命名、File ID、HMAC 和内容替换逻辑；只返回任务内相对目标，无写入/改名/同步删除副作用；API/UI/竞态/容器/浏览器验证 |
| 存储浏览运行当前目录 | ✅ | 认证匹配 API 复用 `Manager.MatchTasks` 的存储/路径边界和最具体任务排序；UI 仅在匹配时显示，防陈旧导航并传递当前目录；竞态、容器、历史和 Chrome 验证 |
| URL 编码与 HMAC 安全签名 | ✅ | QueryEscape 与 `internal/signature` 测试；字面 `%`、`%23`、`%2F` 文件名的生成/API/播放保持一次解码，少编码一层时 HMAC 拒绝，并有竞态和容器 E2E |
| 文件 ID 模式 | 🟡 | 显式配置、驱动能力校验、ID 命名空间签名、播放/删除解析及合成稳定-ID 驱动测试已完成；真实网盘 File ID 驱动与账号验证待完成 |

## Webhook 与联动

| 功能 | 状态 | 当前证据 / 剩余工作 |
|---|---:|---|
| 通用任务 Webhook | ✅ | 支持旧版 `task_id/path` 及公开 `a_task` 路径、增量、保留资产、插件、扫描设置和延迟覆盖；确定性插件顺序、原子多任务入队、CORS Playground、配置不变与删除安全均有竞态/容器 E2E |
| Emby 删除同步 | ✅ | 目标边界、签名 STRM 和真实源删除集成测试 |
| QAS / CloudSaver | ✅ | 支持 `strmtask` 多任务、`savepath`、`xlist_path_fix`、延迟及官方 `refresh=true` OpenList 强刷协议测试 |
| 网页转存事件 | ✅ | 按官方链接用户脚本 v1.2 的 `event=web_save`、`driver=cloud189/quark/open115`、`savepath`、`delay` 实现；最长任务匹配、同级歧义拒绝、OpenList 强刷、响应契约、签名 STRM/302 与历史均有协议/竞态/容器 E2E |
| CloudDrive2 | ✅ | 文档 `file_notify` 负载、最长存储/子路径映射、目录归一化、任务匹配与去重测试 |
| MoviePilot | ✅ | 官方 `transfer.complete` 负载、`target_diritem.path`/`file_list_new`、最长路径映射和任务匹配测试 |
| Token 重置 | ✅ | 32 字节 CSPRNG、一次性 no-store 响应、原子配置更新、UI 和 API 测试 |

## 插件、识别与代理

| 功能 | 状态 | 当前证据 / 剩余工作 |
|---|---:|---|
| 文件名关键词过滤/替换 | ✅ | `filename_skip` 支持字面/正则、exclude/include、大小写与仅目录模式；祖先目录预览一致、旧 `skip_regex` 兼容、Webhook 临时覆盖和严格类型校验均有竞态/容器测试；`replace_regex` 生成测试 |
| STRM 内容替换 | ✅ | 全局/任务生成期正则替换和工具箱批量替换测试 |
| 非法文件名修正 | ✅ | 基础名/扩展名 Windows 非法字符、扩展名之后的尾空白、目录与文件远端改名、扩展名优先保留和 UTF-8 字节上限均有竞态及容器测试 |
| Infuse ISO、自定义 STRM 文件名 | ✅ | 模板生成、目标预检冲突、增量保留、源改名/删除后的同步清理回归测试 |
| 任务请求延时 | ✅ | 列目录和复制后的有界、可取消延时测试 |
| 飞牛影视刷新 | ⬜ | API 配置、库匹配、HTTPS/自签支持待实现 |
| 提取视频封面 | ✅ | 同源签名 STRM 校验、固定 ffmpeg 参数/协议白名单、超时取消、原子 JPG、符号链接防护、预览/API/UI 与假 ffmpeg 协议测试 |
| TMDB 识别与模板 | 🟡 | API/代理、TTL 缓存、搜索、详情、剧集标题、`{title_original}`（movie `original_title`/TV `original_name`，空值回退显示标题）、模板变量提示与同源受限海报代理已实现；隔离协议服务 + 真实 Chrome 完成 movie/TV 搜索 prompt、年份、海报卡片、选择、模板、preview confirm、apply、目录刷新，以及 TV 集标题和空原始标题回退，网络均 200、console/page errors 为空，截图见 `artifacts/ui/tmdb-{movie,tv,fallback}-{results,applied}.png`；缺失 Key 时存储页主动提示且在 prompt 前中止，启动仅注入布尔状态不暴露 Key；真实 TMDB Key/实网限流和地区返回验证仍待完成 |
| 本地/WebDAV 安全流代理 | ✅ | Range、Basic Auth 不泄露和协议测试 |
| OpenList 302 | ✅ | raw URL 校验与重定向测试 |
| Emby/Jellyfin/Plex/飞牛全站 302 代理 | 🟡 | 独立端口、固定上游、安全透传、流式响应、WebSocket、重定向回写和优雅关闭已完成；Emby/Jellyfin PlaybackInfo 已按公开协议做同源 HMAC STRM 改写，并通过 mock、竞态及容器到 OpenList 302 链路；真实客户端及 Plex/飞牛改写待完成 |
| 首播优化、外部播放器、隐藏推广 | ⬜ | 各媒体服务器兼容实现和浏览器验证待完成 |

## 管理、部署与质量

| 功能 | 状态 | 当前证据 / 剩余工作 |
|---|---:|---|
| Basic Auth、Webhook 鉴权、安全响应头 | ✅ | HTTP 集成测试 |
| robots.txt 禁止搜索收录 | ✅ | 公开、无认证挑战的全站 `Disallow: /` 响应，包含 MIME/缓存头与 HTTP/容器/浏览器验证 |
| 任务与存储浏览 UI | ✅ | 任务/工具、存储、实时日志、配置页真实 Chrome 交互；4 张截图、脱敏检查及零控制台/页面/网络错误证据 |
| Docker/Compose、健康检查、资源限制 | ✅ | 构建、Compose 校验和公网健康检查 |
| 配置持久化、备份与迁移 | ✅ | schema v1、旧配置自动迁移、未来版本拒绝、严格校验、0600 原子写、备份恢复、脱敏 API/UI 测试 |
| 日志、运行历史与实时日志 | ✅ | 0600 有界 JSONL、启动恢复、任务生命周期、敏感参数脱敏、筛选 API、SSE 有界订阅；前端按动画帧批量去重渲染且 pending/已显示记录各限制 200 条，避免突发日志逐条重建 DOM |

## 完成规则

不得将 `🟡` 或 `⬜` 项计入完整功能。每次实现后更新本矩阵，并附对应文件、测试、真实协议、浏览器或容器运行证据。
