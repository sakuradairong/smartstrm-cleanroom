## 变更说明

<!-- 说明要解决的问题和实现范围；关联 Issue：Closes #123 -->

## 类型

- [ ] 修复
- [ ] 功能
- [ ] 重构
- [ ] 文档 / CI / 构建

## 验证

<!-- 列出实际执行的命令、协议 mock、Docker 或浏览器证据。 -->

- [ ] `find cmd internal -type f -name '*.go' -print0 | xargs -0 gofmt -w`
- [ ] `go mod tidy`
- [ ] `go vet ./...`
- [ ] `go test -race ./...`
- [ ] `python3 -m json.tool config.example.json >/dev/null`
- [ ] `docker compose config -q`
- [ ] `git diff --check`

## 安全与兼容边界

- [ ] 未提交凭据、Token、真实媒体信息、运行时文件或闭源私有材料。
- [ ] 配置/API/行为变化已同步 README、示例配置和功能矩阵。
- [ ] 破坏性行为有安全默认值、预览和失败保护。
- [ ] 未把 mock 或推测结果描述为真实外部服务兼容。

## 未验证事项

<!-- 明确写出仍缺少的账号、服务版本、客户端或实网证据；没有则写“无”。 -->
