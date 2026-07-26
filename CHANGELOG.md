# Changelog

本项目的重要变更记录在此。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本采用[语义化版本](https://semver.org/lang/zh-CN/)。原闭源项目的公开版本说明仅作为功能审计资料，见 [`docs/UPSTREAM_RELEASE_NOTES.md`](docs/UPSTREAM_RELEASE_NOTES.md)。

## [Unreleased]

### Added

- Go clean-room 单体实现，包含 Local、WebDAV、OpenList 和 ANi Open 存储。
- STRM 生成、调度、Webhook、签名播放、管理 UI、历史、TMDB 命名和媒体联动基础。
- 严格配置、路径约束、远端响应边界、错误脱敏与竞态测试。
- Docker Compose 部署及开源项目社区规范。

- 配置热重载：存储、任务、TMDB、历史修改无需重启即可生效，失败时回滚到上一个可用配置。
- 媒体代理热重载：启用/停用、上游地址、服务器类型和 PlaybackInfo 改写开关即时生效，仅监听端口仍需重启。
- 任务表单可浏览选择源目录与 STRM 输出目录，支持当场新建目录。
- 高级能力配置按功能分组并逐项说明；路径映射改为表格编辑，不再手写 JSON。

### Fixed

- 修复配置热重载与请求并发时的数据竞争（播放、日志流、认证），该竞争可导致进程崩溃。
- 修复启动后立即修改配置文件会被静默忽略、不触发重载的问题。
- 修复任务页、运行日志页直接打开时样式缺失（懒加载分块未包含共享样式）。
- 存储浏览的修改时间不再显示原始 RFC 3339 字符串。
- TMDB 响应缓存增加上限，避免搜索接口导致内存无界增长。
- 修复因未格式化文件导致的 CI 格式检查失败。

### Security

- 同步删除默认关闭；部分任务失败时跳过同步删除。
- 配置、备份、历史和 Token 按敏感数据处理。
- 本地目录浏览接口仅返回目录、解析软链后二次校验，并拒绝新建名中的路径穿越。

[Unreleased]: https://github.com/sakuradairong/smartstrm-cleanroom/commits/main
