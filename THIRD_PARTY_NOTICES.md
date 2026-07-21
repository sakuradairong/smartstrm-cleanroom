# Third-party notices

SmartStrm Clean-room 本身依据 [AGPL-3.0](LICENSE) 发布。第三方组件保留各自版权和许可证；本文件不是其许可证替代品。

## Go 模块

生产二进制当前只使用 Go 标准库。测试依赖可通过以下命令得到精确、可审计清单：

```bash
go list -m -json all
```

直接测试依赖 `github.com/stretchr/testify` 使用 MIT License；其传递依赖的许可证以各模块发行内容为准。`go.mod` 与 `go.sum` 是版本清单的权威来源。

## Web 前端

管理界面构建使用以下直接生产依赖；精确版本以 `web/package-lock.json` 为权威来源：

- React 19.2.7（MIT License）
- ReactDOM 19.2.7（MIT License）
- react-router 7.18.1（MIT License）
- Ant Design 5.29.3（MIT License）
- @ant-design/icons 5.6.1（MIT License）
- dayjs 1.11.21（MIT License）

前端开发与构建工具包括 Vite 6.4.3（MIT）、TypeScript 5.8.3（Apache-2.0）和 Vitest 3.2.7（MIT）。这些工具仅在构建/测试阶段使用，不进入最终 Alpine 运行时镜像；其传递依赖与完整许可证文本以 lockfile 对应发行内容为准。

## 容器组件

镜像基于官方 Go 与 Alpine 镜像，并安装 `ca-certificates`、`tzdata` 和 `ffmpeg`。这些组件不是本项目的一部分，可能包含 Apache-2.0、BSD、GPL、LGPL、MIT 等不同条款。重新分发镜像者应检查所构建架构和镜像摘要对应的 Alpine package license 字段及 ffmpeg 构建配置。

可使用支持 SPDX/CycloneDX 的 SBOM 工具扫描最终镜像；发布流程不应把此人工摘要当作完整 SBOM。

## 外部服务与商标

TMDB、OpenList、Emby、Jellyfin、Plex、CloudDrive2、MoviePilot 和各云存储服务不是本项目的捆绑组件。其名称和商标归各自权利人所有，使用相关服务时应遵守其条款。
