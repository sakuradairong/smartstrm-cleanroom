# 参与贡献

感谢你帮助改进 SmartStrm Clean-room。提交贡献即表示你有权按本仓库的 [AGPL-3.0](LICENSE) 许可证提供这些内容。

## 开始之前

1. 阅读 [README](README.md)、[功能矩阵](docs/FEATURE_MATRIX.md) 和[外部验证要求](docs/EXTERNAL_VALIDATION.md)。
2. Bug 请先搜索现有 Issue，并提供最小复现；安全漏洞请遵循 [SECURITY.md](SECURITY.md)，不要公开披露。
3. 大型功能、协议适配或破坏性变更应先开 Feature request，说明公开协议来源、测试环境和兼容边界。
4. 不要提交闭源 SmartStrm 的反编译结果、私有源码、泄露的 API、第三方凭据、真实媒体信息或无授权素材。

## 本地开发

要求 Go 1.23、Docker Engine 和 Docker Compose v2。封面提取测试需要 `ffmpeg`。

```bash
git clone https://github.com/sakuradairong/smartstrm-cleanroom.git
cd smartstrm-cleanroom
go mod download
go test ./...
```

建议从短生命周期分支提交更改：

```bash
git switch -c feat/short-description
```

## 实现原则

- 保持 clean-room：只依据公开文档、公开标准和自行观测的合法协议行为实现。
- 配置必须严格校验；非法配置不能启动部分后台任务。
- 不猜测私有协议字段，不伪造稳定 File ID，不把 mock 结果描述为真实服务兼容。
- 远端正文、错误、URL 和响应头均是不可信输入；不得泄露 Token、凭据或控制字符。
- 破坏性能力应默认关闭并提供预览；部分失败时不得执行同步删除。
- 新能力需要单元测试；协议集成应附最小可复现实验服务，UI 变更应提供浏览器证据。

## 提交前检查

```bash
find cmd internal -type f -name '*.go' -print0 | xargs -0 gofmt -w
go mod tidy
go vet ./...
go test -race ./...
python3 -m json.tool config.example.json >/dev/null
docker compose config -q
git diff --check
```

若改动涉及 Docker、HTTP、浏览器或第三方协议，还应运行对应的容器化或浏览器回归，并在 Pull Request 中列出命令与结果。测试不得依赖贡献者的私人账号才能通过。

## 提交信息

使用 [Conventional Commits](https://www.conventionalcommits.org/)：

```text
feat(storage): add bounded WebDAV listing
fix(task): skip sync deletion after partial failure
docs: clarify OpenList adapter scope
```

常用类型为 `feat`、`fix`、`docs`、`test`、`refactor`、`build`、`ci` 和 `chore`。一个提交应只解决一个清晰问题。

## Pull Request

- 从 `main` 的最新状态开始，保持改动聚焦。
- 说明问题、实现、风险、验证证据和未验证边界。
- 配置字段、API 或用户行为变化必须同步 README、示例配置和功能矩阵。
- 不要提交生成物、运行时目录、凭据或不相关格式化。
- 维护者可能要求拆分过大的 PR；合并方式由维护者根据历史整洁度决定。

除非有明确测试和证据，否则功能矩阵状态不得提升为“已完成”。
