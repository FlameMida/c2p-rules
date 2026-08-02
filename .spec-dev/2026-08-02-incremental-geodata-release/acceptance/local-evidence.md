# 本地验收证据：有效变化增量发布

> 时间：2026-08-03 CST  
> 验证提交：`06f6d8b1b6177f4541a90349b1655396e0a142ec`  
> 环境：macOS x86_64（Darwin 25.5.0），Go 1.26.5

## Worktree 确定性验证

以下命令均在隔离 worktree 执行并以 exit 0 结束：

```text
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./internal/releasecmp ./internal/verify ./internal/cli
git diff --check
```

全量测试覆盖 `internal/app`、`internal/cli`、`internal/config`、`internal/fetch`、`internal/fileutil`、`internal/geoip`、`internal/geosite`、`internal/manifest`、`internal/model`、`internal/passwall`、`internal/releasecmp`、`internal/rules`、`internal/targets`、`internal/tools`、`internal/verify`、`internal/workspace` 与 `internal/yamlutil`；`cmd/geodata-build` 无独立测试文件。

审查修复期间对以下回归做过临时变异，目标测试均先红后绿，变异全部恢复且未进入提交：

- FirstRelease / Force 候选安装器 tag 与上下文错绑；
- build 404 `case` 改为固定值、publish 404 比较符反转；
- current ID/tag/fingerprint 改为期望值自赋值或把真实来源移到比较之后；
- 默认分支门禁 `&&` 改为 `||`；
- 严格 `$readback` 校验移到 Release 公开之后或与另一条命令拆开；
- first-release 404 复核块移入错误的 outer case 分支。

## 干净归档验收

从验证提交执行 `git archive HEAD`，解包到 `/tmp/clash-rules-srs-acceptance.TXKLza`；归档不包含 worktree 未跟踪文件，也未额外安装 Python、Node 或 npm。

以下步骤通过：

```text
go run ./cmd/geodata-build bootstrap
# exit 0

go test -count=1 ./...
# exit 0；17 个有测试的 internal package 全部通过

go test -count=1 -tags=integration ./internal/app
# exit 0；ok clash-rules-srs/internal/app
```

完整构建尝试：

```text
go run ./cmd/geodata-build build \
  --repo flame/clash-rules-srs \
  --release-tag geodata-acceptance-06f6d8b
```

结果：`exit 1`。失败发生在读取外部 GitHub raw 上游时，尚未进入产物生成：

```text
ERROR: fetch and parse sources: source "loyalsoldier-reject": read fetch response https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/reject.txt: read tcp ...:443: read: operation timed out
exit status 1
```

因此“clean archive 完整 build + 严格六资产”保持 `unverified`；bootstrap、全量测试与真实工具 integration 的通过不能替代该项，也不把外部网络超时记为产品失败。

## 外部边界

- 当前 checkout 没有 Git remote，未获得指定可写仓库与 Release 写入授权；未创建真实 draft/tag/Release。
- 当前没有可触发并读取日志的 GitHub-hosted runner；未执行连续两次可信 latest 的远端无变化运行。
- 上述两项按已批准边界记为 `DEFERRED`，解除条件分别是可写测试仓库授权，以及可触发并读取 Actions/Artifact/Release 状态的 runner 权限。

