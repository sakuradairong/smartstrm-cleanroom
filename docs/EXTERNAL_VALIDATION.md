# 外部联调与解除阻塞清单

本文只记录当前 clean-room 实现无法在本仓库内自行取得的输入。提供凭据时应通过本地配置文件或环境变量注入，不要提交到 Git、聊天记录、截图或测试夹具。

## 原生云盘驱动与 File ID

每个目标供应商需要：

1. 官方或明确授权的 API 文档、OAuth/设备授权流程和使用条款。
2. 独立测试账号；账号中包含可浏览、下载、重命名、移动和删除的测试目录。
3. 可验证原画/转码直链、Range、过期刷新和稳定 File ID 的媒体样例。
4. 对限频、刷新令牌、家庭云/CAS 等供应商特性的明确测试预期。

在这些条件满足前，夸克、115、天翼和 123 继续通过 OpenList 接入；不会逆向移动端请求或把路径/URL 哈希当成稳定 File ID。

## TMDB

需要：

- TMDB API Key；
- 如适用，HTTP(S) 代理 URL；
- 允许执行电影、剧集、单集详情、海报、缓存命中/过期、错误、限流和语言差异测试的网络环境。

当前 mock 测试覆盖协议和安全边界，但不能证明真实服务可用。

## 媒体服务器与飞牛影视

每种服务器需要固定版本、隔离测试库和至少一个由 SmartStrm 生成的 STRM 样例。验证范围包括：

- Web、官方客户端和至少一个第三方客户端的直接播放与转码；
- 播放 API 的路径、Query、认证头、Range、重定向和 WebSocket；
- Emby/Jellyfin 公开 PlaybackInfo 改写的真实版本回归，以及 Plex/飞牛的服务器特定 STRM API 改写；
- 首播优化的正常路径和回退路径；
- 外部播放器按钮、网页注入和隐藏推广的实际 DOM/浏览器回归；
- 飞牛刷新 API、HTTPS 和自签证书策略。

固定上游反向代理基座已经通过 mock、WebSocket、容器和 SIGTERM 在途请求排空测试。Emby/Jellyfin 的 `/Items/{Id}/PlaybackInfo` 改写已验证真实 HMAC、未知字段保留、客户端认证透传和签名流到 OpenList 302 的完整容器链路；仍需真实 Emby/Jellyfin 版本和客户端验证。Plex/飞牛改写尚未实现。

## 网页转存事件

官方公开 Webhook 文档指向的 “SmartStrm 助手 - 转存触发任务” v1.2 用户脚本已审计并实现。协议范围严格限定为 `event=web_save`、`delay`、`data.driver`（`cloud189`、`quark`、`open115`）和 `data.savepath`；响应兼容脚本读取的 `success`、`message` 和 `task.name/storage_path`。其他网盘或未来脚本字段需要新的公开版本证据，不会提前猜测。

## 完成判定

取得上述输入后，每个集成仍需提供最新的单元、协议 mock、竞态、真实服务、容器端到端和（适用时）浏览器/客户端证据。缺少任一真实外部验证的项目继续在 [`FEATURE_MATRIX.md`](FEATURE_MATRIX.md) 中保持 `🟡` 或 `⬜`。
