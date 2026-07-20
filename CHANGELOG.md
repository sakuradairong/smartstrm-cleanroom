# Changelog

本项目的重要变更记录在此。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本采用[语义化版本](https://semver.org/lang/zh-CN/)。原闭源项目的公开版本说明仅作为功能审计资料，见 [`docs/UPSTREAM_RELEASE_NOTES.md`](docs/UPSTREAM_RELEASE_NOTES.md)。

## [Unreleased]

### Added

- Go clean-room 单体实现，包含 Local、WebDAV、OpenList 和 ANi Open 存储。
- STRM 生成、调度、Webhook、签名播放、管理 UI、历史、TMDB 命名和媒体联动基础。
- 严格配置、路径约束、远端响应边界、错误脱敏与竞态测试。
- Docker Compose 部署及开源项目社区规范。

### Security

- 同步删除默认关闭；部分任务失败时跳过同步删除。
- 配置、备份、历史和 Token 按敏感数据处理。

[Unreleased]: https://github.com/sakuradairong/smartstrm-cleanroom/commits/main
