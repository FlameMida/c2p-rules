# 全 Go geodata 构建与 PassWall2 分流安装实施计划

> **执行方式**：使用 spec-dev 的 executing-plans skill 逐任务执行本计划；无该 skill 的环境直接从任务 0 起按序执行至最终任务。步骤用复选框（`- [ ]`）语法跟踪；脱离项目携带时连同特性目录（含 spec）整体带走。
>
> **偏差处理**：执行中发现计划与现实不符——小偏差（路径笔误、明显遗漏但意图清楚）就地修正并在提交信息中注明；接口、数据结构等契约级偏差停下向计划作者确认，不猜着改。

**目标**：把 Python/Node 构建链替换为单一 Go CLI，按显式 source/output 契约合并远程和本地规则，并发布可事务安装的 PassWall2 分流脚本。

**Spec**：`.spec-dev/2026-07-31-go-geodata-pipeline/spec/go-geodata-pipeline-design.md`

**架构**：根目录 Go module 以 `cmd/geodata-build` 为唯一第一方入口，业务代码全部置于 `internal/`；`domain-list-custom`、`geoip`、`geoview` 继续作为固定提交的受控子进程。构建在 staging 中完成 dat、manifest、installer 和三份校验文件的全部验证后，再切换 `build/` 与 `publish/`。

**技术栈**：Go 1.26.x、`go.yaml.in/yaml/v3`、标准库 `net/netip`/`net/http`/`os/exec`、固定提交的 domain-list-custom/geoip/geoview、BusyBox `/bin/sh`、OpenWrt UCI、GitHub Actions。

## 全局约束

- 生产代码无 Python、Node、npm 运行时；生成的 PassWall2 installer 是唯一保留的第一方 shell 交付物。
- 根目录使用单一 Go module，业务包全部在 `internal/`，不建立未经需求承诺的公共 `pkg/` API。
- `domain-list-custom` 固定 `efacb51b8950ae673ebb6dcb9e7ecdd1decb1b6d`，`geoip` 固定 `85084dfbe282e4e9cb460b07196e6eecfd126d19`，`geoview` 固定 `3c91926d360b8f49d47520639e574608318baf12`。
- `sources.yaml` 只接受 `id/behavior/format/url/outputs`；拒绝旧 `name+sides` schema。
- Geosite 只做同 tag 规范化精确去重；不同类型、属性、keyword、regexp 保留，不做跨 tag 差集。
- GeoIP 使用 pinned geoip 的 IPSet 并集语义；base input 永远在 config 第一项。
- `BilibiliHMT` 大小写保真并与普通 `bilibili` 隔离；YouTube 托管分流必须高于 Google。
- Release 资产精确为两个 dat、两个 dat sha、installer、installer sha，共六项。
- installer 只迁移旧 `c2p_` 和 `managed_by=clash-rules-srs` section，保留用户分流、节点和其他 section；初装用不可变 tag，成功后持久化 `latest` URL。
- 每个实现任务执行红—绿—重构循环；没有观察到失败测试不得编写该任务的生产代码。

## 文件结构与职责

```text
go.mod / go.sum                         Go module 与固定直接依赖
cmd/geodata-build/main.go               唯一 CLI 进程入口
internal/model/model.go                 Side/Mode/Source/Group/Buckets 等共享值类型
internal/cli/run.go                     子命令解析、退出码、依赖注入
internal/config/yaml.go                 严格 YAML、重复 key、多 document 防护
internal/config/sources.go              sources schema 与跨字段校验
internal/config/groups.go               PassWall groups schema 与顺序校验
internal/rules/parse.go                 yaml/text payload 和 Clash rule 分类
internal/rules/custom.go                custom 目录发现与逐 side 校验
internal/targets/registry.go            base/create/merge-base/final tag registry
internal/geosite/merge.go               community + contributions 安全合并
internal/geoip/config.go                CIDR 输入与 geoip converter config
internal/fetch/client.go                有 deadline、redirect、大小上限的下载器
internal/tools/runner.go                固定二进制、CommandContext、有界日志
internal/tools/bootstrap.go             固定 checkout 与三个工具编译
internal/manifest/manifest.go           required/forbidden/provenance JSON
internal/verify/geoview.go              正负 tag、groups 引用和资产探针
internal/workspace/transaction.go        staging 与 build/publish 可恢复切换
internal/passwall/render.go              groups → 稳定具名 UCI fragment
internal/passwall/installer.go           事务安装 shell renderer
internal/passwall/install.sh.tmpl        BusyBox-compatible shell 模板
internal/app/build.go                    完整 build use-case 编排
internal/app/bootstrap.go                bootstrap use-case
internal/app/verify.go                   verify use-case
config/geoip.base.json                   pinned geoip CLI 模板
config/passwall2-groups.yaml             默认逻辑服务与优先级真源
custom/geosite/apple.yaml                带注释的空 geosite 模板
custom/geoip/cn.yaml                     带注释的空 geoip 模板
sources.yaml                             18 个 source 的显式 outputs
internal/**/testdata/                    synthetic fixtures 与 golden
```

---

### 任务 0：建立隔离工作区

- [x] **步骤 1：检测已有隔离**

运行：`git rev-parse --git-dir` 与 `git rev-parse --git-common-dir`
两者不同、且 `git rev-parse --show-superproject-working-tree` 无输出（排除 submodule）
→ 已在隔离工作区，跳过本任务。

- [x] **步骤 2：建立 worktree**

Codex 直接走手工路径：先运行 `git check-ignore -q .worktrees`，再运行：

```bash
git worktree add .worktrees/go-geodata-pipeline -b plan/2026-07-31-go-geodata-pipeline
cd .worktrees/go-geodata-pipeline
```

若 `.worktrees/` 未被忽略，先把 `.worktrees/` 加入 `.gitignore`，提交 `chore(T0): 忽略计划工作区`，再建立 worktree。

- [x] **步骤 3：安装依赖并验证基线**

当前实现尚未有 Go module，先运行旧基线：

```bash
python -m unittest discover -s tests -v
npm --prefix tools/clash2passwall ci --ignore-scripts
npm --prefix tools/clash2passwall test
```

预期：Python 与 Node 测试全部通过。失败则停止并报告，不把基线失败带入 Go 重写。

## 基础与输入契约

### 任务 1：建立 Go module、共享模型与可测试 CLI 壳

**文件**：
- 创建：`go.mod`
- 创建：`go.sum`
- 创建：`internal/model/model.go`
- 创建：`internal/cli/run.go`
- 创建：`internal/cli/run_test.go`
- 创建：`cmd/geodata-build/main.go`

**接口**：
- 消费：无。
- 产出：`model.Side`、`model.Mode`、`model.Source`、`model.Group`、`model.Buckets`、`cli.Commands`、`cli.UsageError`、`cli.Run(context.Context, []string, io.Writer, io.Writer, Commands) int`。

- [x] **步骤 1：写失败测试**

创建 `internal/cli/run_test.go`：

```go
package cli_test

import (
	"bytes"
	"context"
	"testing"

	"clash-rules-srs/internal/cli"
)

func TestHelpIsSuccessful(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), []string{"--help"}, &out, &errOut, cli.Commands{})
	if code != 0 || out.String() == "" || errOut.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), []string{"unknown"}, &out, &errOut, cli.Commands{})
	if code != 2 || errOut.String() != "ERROR: unknown command: unknown\n" {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}
```

- [x] **步骤 2：运行测试确认失败**

运行：`go test ./internal/cli -run 'Test(Help|Unknown)' -v`

预期：FAIL，报 `go: cannot find main module` 或缺少 `internal/cli`。

- [x] **步骤 3：写最小实现**

`go.mod`：

```go
module clash-rules-srs

go 1.26

require go.yaml.in/yaml/v3 v3.0.5
```

`internal/model/model.go` 至少定义：

```go
package model

import "net/netip"

type Side string
const (
	GeoSite Side = "geosite"
	GeoIP   Side = "geoip"
)

type Mode string
const (
	Create    Mode = "create"
	MergeBase Mode = "merge-base"
)

type Behavior string
const (
	Domain    Behavior = "domain"
	IPCIDR    Behavior = "ipcidr"
	Classical Behavior = "classical"
)

type Format string
const (
	YAML Format = "yaml"
	Text Format = "text"
)

type Output struct { Tag string; Mode Mode }
type Outputs struct { GeoSite *Output; GeoIP *Output }
type Source struct { ID string; Behavior Behavior; Format Format; URL string; Outputs Outputs }
type Group struct { ID, Remarks string; GeoSite, GeoIP []string }
type DomainRule struct { Kind, Value string; Attrs []string }
type Buckets struct { Domains []DomainRule; CIDRs []netip.Prefix; Skipped []string }
type Contribution struct { SourceID string; Side Side; Tag string; Domains []DomainRule; CIDRs []netip.Prefix }
```

`internal/cli/run.go`：

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
)

type Command func(context.Context, []string, io.Writer, io.Writer) error
type Commands struct { Bootstrap, Build, Verify Command }
type UsageError struct { Err error }
func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func Run(ctx context.Context, args []string, out, errOut io.Writer, commands Commands) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: geodata-build <bootstrap|build|verify> [options]")
		return 0
	}
	var command Command
	switch args[0] {
	case "bootstrap": command = commands.Bootstrap
	case "build": command = commands.Build
	case "verify": command = commands.Verify
	default:
		fmt.Fprintf(errOut, "ERROR: unknown command: %s\n", args[0])
		return 2
	}
	if command == nil {
		fmt.Fprintf(errOut, "ERROR: command not wired: %s\n", args[0])
		return 1
	}
	if err := command(ctx, args[1:], out, errOut); err != nil {
		fmt.Fprintf(errOut, "ERROR: %v\n", err)
		var usage *UsageError
		if errors.As(err, &usage) { return 2 }
		return 1
	}
	return 0
}
```

`cmd/geodata-build/main.go` 调用 `cli.Run` 并暂以空 `cli.Commands{}` 接线，任务 11 再注入 app commands。

- [x] **步骤 4：运行测试确认通过**

运行：`go mod tidy && go test ./internal/cli ./cmd/geodata-build -v`

预期：PASS；`go.sum` 生成，主程序可编译。

- [x] **步骤 5：提交**

```bash
git add go.mod go.sum internal/model internal/cli cmd/geodata-build
git commit -m "feat(T1): 建立 Go 模型与 CLI 入口"
```

### 任务 2：实现严格 sources/groups YAML 与输出契约

**文件**：
- 创建：`internal/config/yaml.go`
- 创建：`internal/config/sources.go`
- 创建：`internal/config/groups.go`
- 创建：`internal/config/config_test.go`
- 创建：`internal/config/testdata/sources-valid.yaml`
- 创建：`internal/config/testdata/groups-valid.yaml`

**接口**：
- 消费：任务 1 的 `model.Source`、`model.Group`。
- 产出：`config.ParseSources(io.Reader) ([]model.Source, error)`、`config.LoadSources(string) ([]model.Source, error)`、`config.ParseGroups(io.Reader) ([]model.Group, error)`、`config.LoadGroups(string) ([]model.Group, error)`、`config.DecodeStrict(io.Reader, any) error`。

- [x] **步骤 1：写失败测试**

在 `internal/config/config_test.go` 写 table tests，至少包含：

```go
func TestSourcesRejectLegacySidesAndDuplicateKeys(t *testing.T) {
	for name, document := range map[string]string{
		"legacy": "sources:\n- id: x\n  behavior: domain\n  url: https://e.test/x\n  sides: [geosite]\n",
		"duplicate": "sources:\n- id: x\n  id: y\n  behavior: domain\n  url: https://e.test/x\n  outputs:\n    geosite: {tag: x, mode: create}\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.ParseSources(strings.NewReader(document))
			if err == nil { t.Fatal("expected strict YAML error") }
		})
	}
}

func TestGoogleUsesExplicitMergeBaseTarget(t *testing.T) {
	sources, err := config.ParseSources(strings.NewReader(`sources:
- id: loyalsoldier-google
  behavior: domain
  url: https://example.test/google
  outputs:
    geosite: {tag: google, mode: merge-base}
`))
	if err != nil { t.Fatal(err) }
	got := sources[0]
	if got.ID != "loyalsoldier-google" || got.Outputs.GeoSite.Tag != "google" || got.Outputs.GeoSite.Mode != model.MergeBase {
		t.Fatalf("unexpected source: %#v", got)
	}
}

func TestGroupsPreserveOrderAndRejectMissingSides(t *testing.T) {
	groups, err := config.ParseGroups(strings.NewReader(`groups:
- id: youtube
  remarks: YouTube
  geosite: [youtube]
  geoip: []
- id: google
  remarks: Google 服务
  geosite: [google]
  geoip: []
`))
	if err != nil { t.Fatal(err) }
	if groups[0].ID != "youtube" || groups[1].ID != "google" { t.Fatalf("order=%v", groups) }
}
```

另加非法 tag、未知 mode、重复 source ID、重复 group ID、第二个 YAML document、非字符串 scalar 和控制字符用例。

- [x] **步骤 2：运行测试确认失败**

运行：`go test ./internal/config -v`

预期：FAIL，报 `undefined: config.ParseSources` / `ParseGroups`。

- [x] **步骤 3：写最小实现**

`internal/config/yaml.go` 使用 `yaml.Node` 递归拒绝同一 mapping 的重复 key，再用 `yaml.Decoder.KnownFields(true)` 解码 typed document，并第二次 `Decode` 必须得到 `io.EOF`。

`internal/config/sources.go` 使用私有 YAML DTO：

```go
type sourceYAML struct {
	ID string `yaml:"id"`
	Behavior model.Behavior `yaml:"behavior"`
	Format model.Format `yaml:"format,omitempty"`
	URL string `yaml:"url"`
	Outputs struct {
		GeoSite *outputYAML `yaml:"geosite,omitempty"`
		GeoIP *outputYAML `yaml:"geoip,omitempty"`
	} `yaml:"outputs"`
}
type outputYAML struct { Tag string `yaml:"tag"`; Mode model.Mode `yaml:"mode"` }
```

验证规则：ID/tag 匹配 `^[A-Za-z0-9][A-Za-z0-9._-]*$`；format 空值归一为 `yaml`；behavior/format/mode 必须在枚举内；URL 必须为 HTTPS；source ID 唯一；至少一个 output；domain 只能声明 geosite、ipcidr 只能声明 geoip、classical 可声明一侧或两侧。

`internal/config/groups.go` 要求 `id` 匹配 `^[a-z][a-z0-9_]{0,47}$`，remarks 非空且无控制字符，两个 side 字段都必须出现，group ID 唯一，每侧 tag 列表去重。

- [x] **步骤 4：运行测试确认通过**

运行：`go test ./internal/config -v`

预期：PASS，所有严格 YAML 与顺序用例通过。

- [x] **步骤 5：提交**

```bash
git add internal/config
git commit -m "feat(T2): 定义严格 source 与分流配置"
```

### 任务 3：实现 Clash 规则解析与 custom 目录加载

**文件**：
- 创建：`internal/rules/parse.go`
- 创建：`internal/rules/custom.go`
- 创建：`internal/rules/parse_test.go`
- 创建：`internal/rules/custom_test.go`
- 创建：`internal/rules/testdata/custom/geosite/BilibiliHMT.yaml`
- 创建：`internal/rules/testdata/custom/geoip/netflix.yaml`

**接口**：
- 消费：任务 1 的 `model.Behavior`、`model.Format`、`model.Buckets`、`model.Contribution`。
- 产出：`rules.Parse(io.Reader, model.Format, model.Behavior) (model.Buckets, error)`、`rules.LoadCustom(string, TargetChecker) ([]model.Contribution, error)`；`TargetChecker` 为 `Require(model.Side, string) error`。

- [x] **步骤 1：写失败测试**

`internal/rules/parse_test.go` 直接翻译规则映射与 Netflix 双侧场景：

```go
func TestParseClassicalNetflixSplitsDomainAndCIDR(t *testing.T) {
	in := `payload:
  - DOMAIN-SUFFIX,netflix.com
  - DOMAIN,api.netflix.com
  - DOMAIN-KEYWORD,nflx
  - DOMAIN-REGEX,^.+\.nflxvideo\.net$
  - IP-CIDR,23.246.0.0/18,no-resolve
  - IP-CIDR6,2001:db8::/32,no-resolve
`
	b, err := rules.Parse(strings.NewReader(in), model.YAML, model.Classical)
	if err != nil { t.Fatal(err) }
	if len(b.Domains) != 4 || len(b.CIDRs) != 2 { t.Fatalf("buckets=%#v", b) }
	if b.CIDRs[0] != netip.MustParsePrefix("23.246.0.0/18") { t.Fatalf("cidr=%v", b.CIDRs) }
}
```

`internal/rules/custom_test.go` 覆盖 Spec 的两个 custom 场景：

```go
type checker map[model.Side]map[string]bool
func (c checker) Require(side model.Side, tag string) error {
	if c[side][tag] { return nil }
	return fmt.Errorf("unknown target %s:%s", side, tag)
}

func TestCustomCanExtendCreatedBilibiliHMT(t *testing.T) {
	got, err := rules.LoadCustom("testdata/custom", checker{
		model.GeoSite: {"BilibiliHMT": true}, model.GeoIP: {"netflix": true},
	})
	if err != nil { t.Fatal(err) }
	if got[0].Tag != "BilibiliHMT" || got[0].Domains[0].Value != "example.test" { t.Fatalf("got=%#v", got) }
}

func TestCustomRejectsUnknownTargetBeforeEmission(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "geosite"), 0o755)
	os.WriteFile(filepath.Join(dir, "geosite", "googel.yaml"), []byte("payload:\n  - DOMAIN-SUFFIX,example.test\n"), 0o644)
	_, err := rules.LoadCustom(dir, checker{})
	if err == nil || !strings.Contains(err.Error(), "geosite:googel") { t.Fatalf("err=%v", err) }
}
```

另测 geosite 文件含 IP、geoip 文件含 DOMAIN、非法 CIDR、`no-resolve` 之外第三字段、空模板为语义空集。

- [x] **步骤 2：运行测试确认失败**

运行：`go test ./internal/rules -v`

预期：FAIL，报缺少 `rules.Parse` / `LoadCustom`。

- [x] **步骤 3：写最小实现**

`Parse` 对 YAML 只接受 mapping root 与 `payload: []string|null`，对 text 按非空非注释行读取；分类映射固定为：

```go
var domainKinds = map[string]string{
	"DOMAIN-SUFFIX": "domain",
	"DOMAIN": "full",
	"DOMAIN-KEYWORD": "keyword",
	"DOMAIN-REGEX": "regexp",
}
```

CIDR 用 `netip.ParsePrefix` 后 `.Masked()`；`IP-SUFFIX`、PROCESS 家族及其他未知类型进入 `Skipped`，但 custom loader 对未知类型直接报错。`LoadCustom` 只扫描 `geosite/*.yaml` 与 `geoip/*.yaml`，按完整路径排序，文件 stem 保持大小写，调用 `TargetChecker.Require` 后才解析并生成 contribution。

- [x] **步骤 4：运行测试确认通过**

运行：`go test ./internal/rules -v`

预期：PASS；Netflix 六条输入分到 4 条域名与 2 条 CIDR，unknown custom target 报错含 side/tag。

- [x] **步骤 5：提交**

```bash
git add internal/rules
git commit -m "feat(T3): 解析远程与本地 Clash 规则"
```

## 合并与构建核心

### 任务 4：实现 target registry 与 Geosite 安全合并

**文件**：
- 创建：`internal/targets/registry.go`
- 创建：`internal/targets/registry_test.go`
- 创建：`internal/geosite/merge.go`
- 创建：`internal/geosite/merge_test.go`
- 创建：`internal/geosite/testdata/community/google`
- 创建：`internal/geosite/testdata/community/youtube`

**接口**：
- 消费：任务 1 的 `model.Source`/`Contribution`，任务 2 的 sources，任务 3 的 domain rules。
- 产出：`targets.New([]model.Source, BaseLookup) (*Registry, error)`、`(*Registry).Require(model.Side, string) error`、`geosite.Merge(string, string, []model.Contribution) error`。

- [x] **步骤 1：写失败测试**

`internal/targets/registry_test.go`：

```go
func TestCreateAndMergeBasePreconditions(t *testing.T) {
	base := func(side model.Side, tag string) (bool, error) { return tag == "google", nil }
	for name, output := range map[string]model.Output{
		"create-collision": {Tag: "google", Mode: model.Create},
		"merge-missing": {Tag: "missing", Mode: model.MergeBase},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := targets.New([]model.Source{{ID: name, Outputs: model.Outputs{GeoSite: &output}}}, base)
			if err == nil || !strings.Contains(err.Error(), name) { t.Fatalf("err=%v", err) }
		})
	}
}
```

`internal/geosite/merge_test.go`：

```go
func TestMergeDeduplicatesExactRulesButPreservesKindAndAttrs(t *testing.T) {
	out := t.TempDir()
	inputs := []model.Contribution{{SourceID: "custom", Side: model.GeoSite, Tag: "google", Domains: []model.DomainRule{
		{Kind: "domain", Value: "example.com"},
		{Kind: "full", Value: "example.com"},
		{Kind: "domain", Value: "example.com", Attrs: []string{"@cn"}},
	}}}
	if err := geosite.Merge("testdata/community", out, inputs); err != nil { t.Fatal(err) }
	text, _ := os.ReadFile(filepath.Join(out, "google"))
	if strings.Count(string(text), "domain:example.com\n") != 1 || !bytes.Contains(text, []byte("full:example.com")) || !bytes.Contains(text, []byte("@cn")) {
		t.Fatalf("merged=%s", text)
	}
}
```

另测：未声明 community collision 在 registry 阶段失败；create 输出生成新文件；Google 保留 `domain:googleapis.com`、YouTube 保留 `full:youtubei.googleapis.com`；合并后的 `BilibiliHMT` 不修改 `bilibili`。

- [x] **步骤 2：运行测试确认失败**

运行：`go test ./internal/targets ./internal/geosite -v`

预期：FAIL，报缺少 registry/merge 实现。

- [x] **步骤 3：写最小实现**

`BaseLookup` 精确签名：

```go
type BaseLookup func(model.Side, string) (bool, error)
type Registry struct { final map[model.Side]map[string]struct{}; lookup BaseLookup }
```

`targets.New` 对每个 output 调用 base lookup：create 要求 false，merge-base 要求 true；注册全部最终 tag，并拒绝同 side/tag 上互相冲突的 mode。`Registry.Require` 先查 final，再延迟查询底包，使 custom 可以扩展未被远程 source 引用但底包已有的 tag。

`geosite.Merge` 先复制 community 文件到新输出目录，再按 `(tag, SourceID)` 排序 contributions；每个目标将 base 原行与编码后的 `domain/full/keyword/regexp` 行合并。canonical key 为 `kind + NUL + value + NUL + 完整 attrs`；注释、include 和无法归类的 community 行原样保留，不执行父域覆盖删除。

- [x] **步骤 4：运行测试确认通过**

运行：`go test ./internal/targets ./internal/geosite -v`

预期：PASS；精确重复删除，full/属性规则保留，Google/YouTube 与 Bilibili fixtures 隔离。

- [x] **步骤 5：提交**

```bash
git add internal/targets internal/geosite
git commit -m "feat(T4): 实现显式目标与 Geosite 合并"
```

### 任务 5：实现 GeoIP 输入、CIDR 规范化与 converter config

**文件**：
- 创建：`internal/geoip/config.go`
- 创建：`internal/geoip/config_test.go`
- 修改：`config/geoip.base.json`

**接口**：
- 消费：任务 3 的 CIDR contributions。
- 产出：`geoip.WriteInputs(string, []model.Contribution) ([]geoip.Input, error)`、`geoip.WriteConfig(string, []geoip.Input, string, string, string) error`。

- [x] **步骤 1：写失败测试**

创建 `internal/geoip/config_test.go`：

```go
func TestWriteConfigKeepsBaseFirstAndSortsTargets(t *testing.T) {
	dir := t.TempDir()
	inputs, err := geoip.WriteInputs(dir, []model.Contribution{
		{SourceID: "z", Side: model.GeoIP, Tag: "netflix", CIDRs: []netip.Prefix{netip.MustParsePrefix("23.246.0.0/18")}},
		{SourceID: "a", Side: model.GeoIP, Tag: "BilibiliHMT", CIDRs: []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}},
	})
	if err != nil { t.Fatal(err) }
	path := filepath.Join(dir, "config.json")
	if err := geoip.WriteConfig("../../config/geoip.base.json", inputs, "/tmp/base.dat", dir, path); err != nil { t.Fatal(err) }
	var got struct{ Input []struct{ Type string `json:"type"`; Args struct{ Name, URI string `json:"name","uri"` } `json:"args"` } `json:"input"` }
	decodeJSON(t, path, &got)
	if got.Input[0].Type != "v2rayGeoIPDat" || got.Input[1].Args.Name != "BilibiliHMT" || got.Input[2].Args.Name != "netflix" { t.Fatalf("input=%#v", got.Input) }
}

func TestCIDRsAreMaskedSortedAndDeduplicated(t *testing.T) {
	dir := t.TempDir()
	inputs, err := geoip.WriteInputs(dir, []model.Contribution{{Side: model.GeoIP, Tag: "netflix", CIDRs: []netip.Prefix{
		netip.MustParsePrefix("23.246.0.1/18"), netip.MustParsePrefix("23.246.0.0/18"), netip.MustParsePrefix("23.246.0.0/19"),
	}}})
	if err != nil { t.Fatal(err) }
	got, _ := os.ReadFile(inputs[0].Path)
	if string(got) != "23.246.0.0/18\n23.246.0.0/19\n" { t.Fatalf("got=%q", got) }
}
```

- [x] **步骤 2：运行测试确认失败**

运行：`go test ./internal/geoip -v`

预期：FAIL，报缺少 `WriteInputs` / `WriteConfig`。

- [x] **步骤 3：写最小实现**

`Input`：

```go
type Input struct { Tag, Path string }
```

`WriteInputs` 按 tag 聚合所有 CIDR，调用 `Prefix.Masked()`，以 family/address/prefix bits 排序，删除完全重复项，写 `<dir>/<tag>.txt`。不在此层删除被父 prefix 覆盖的子 prefix，该语义由 pinned geoip 的 IPSet 完成。

`WriteConfig` 解析 `config/geoip.base.json`，把第一项 URI 改为本地 base dat 绝对路径，按 tag 排序追加 text/action:add inputs，并把 outputDir/outputName 设为 staging publish 与 `geoip.dat`；未知或缺失模板字段直接报错。

- [x] **步骤 4：运行测试确认通过**

运行：`go test ./internal/geoip -v`

预期：PASS；base 永远第一，BilibiliHMT 大小写保留，CIDR 文本稳定。

- [x] **步骤 5：提交**

```bash
git add internal/geoip config/geoip.base.json
git commit -m "feat(T5): 生成规范 GeoIP 合并输入"
```

### 任务 6：实现受控下载、固定工具 runner 与 bootstrap

**文件**：
- 创建：`internal/fetch/client.go`
- 创建：`internal/fetch/client_test.go`
- 创建：`internal/tools/runner.go`
- 创建：`internal/tools/runner_test.go`
- 创建：`internal/tools/bootstrap.go`
- 创建：`internal/tools/bootstrap_test.go`

**接口**：
- 消费：任务 1 的 context/CLI 基础。
- 产出：`fetch.New(fetch.Options) *fetch.Client`、`(*fetch.Client).Get(context.Context, string, int64) ([]byte, error)`、`(*tools.Runner).Run(context.Context, string, string, ...string) error`、`tools.Executor`（`Run(context.Context, string, string, ...string) error`，参数依次为 cwd/program/args）、`tools.Bootstrap(context.Context, string, tools.Executor) error`、固定 `tools.Pins`。

- [ ] **步骤 1：写失败测试**

`internal/fetch/client_test.go` 使用 `httptest.Server` 覆盖成功、404、总超时、Content-Length 超限、chunked 超限、redirect loop；关键测试：

```go
func TestGetRejectsChunkedBodyOverLimit(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		io.WriteString(w, strings.Repeat("x", 9))
	}))
	defer s.Close()
	client := fetch.New(fetch.Options{Timeout: time.Second, AllowHTTPForTests: true})
	_, err := client.Get(context.Background(), s.URL, 8)
	if err == nil || !strings.Contains(err.Error(), "limit 8") { t.Fatalf("err=%v", err) }
}
```

`internal/tools/runner_test.go`：

```go
func TestRunnerUsesOnlyConfiguredBinRoot(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "geoview"), "#!/bin/sh\nprintf '%s' \"$0\" > \"$1\"\n")
	out := filepath.Join(t.TempDir(), "used-path")
	r := tools.Runner{BinRoot: bin, Timeout: time.Second, MaxLogBytes: 4096}
	if err := r.Run(context.Background(), "geoview", "", out); err != nil { t.Fatal(err) }
	got, err := os.ReadFile(out)
	if err != nil { t.Fatal(err) }
	if string(got) != filepath.Join(bin, "geoview") { t.Fatalf("path=%q", got) }
}
```

bootstrap 测试给 fake `Executor`，断言三个工具使用精确 commit，domain-list-custom/geoip/geoview 都执行 `go build -o .cache/bin/<name>`，community 执行滚动 `fetch origin HEAD` 并记录 HEAD。

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/fetch ./internal/tools -v`

预期：FAIL，报缺少 Client/Runner/Bootstrap。

- [ ] **步骤 3：写最小实现**

`fetch.Options` 固定包含 `Timeout`、`DialTimeout`、`TLSHandshakeTimeout`、`ResponseHeaderTimeout`、`MaxRedirects` 和测试专用 HTTP 开关；生产 URL 只接受 HTTPS，redirect 禁止从 HTTPS 降级。`Get` 先检查状态与 Content-Length，再用 `io.LimitReader(body, max+1)`，超限或非 2xx 返回含 URL 的稳定错误。

`tools.Runner.Run`：

```go
func (r *Runner) Run(ctx context.Context, name, cwd string, args ...string) error {
	tool := filepath.Join(r.BinRoot, name)
	if st, err := os.Stat(tool); err != nil || st.IsDir() { return fmt.Errorf("tool %s missing at %s", name, tool) }
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = cwd
	var stdout, stderr cappedBuffer
	stdout.N, stderr.N = r.MaxLogBytes, r.MaxLogBytes
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil { return fmt.Errorf("tool %s failed: %w; stderr=%s", name, err, stderr.String()) }
	return nil
}
```

`tools.Pins` 写入全局约束三个完整 commit；bootstrap checkout 根为 `<cache>/upstream`、bin 为 `<cache>/bin`，所有 git/go 调用通过可测试 `Executor` 参数数组执行，不调用 shell。

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./internal/fetch ./internal/tools -v`

预期：PASS；超限响应、未知 PATH 工具和错误 commit 均被测试阻断。

- [ ] **步骤 5：提交**

```bash
git add internal/fetch internal/tools
git commit -m "feat(T6): 固定下载与上游工具边界"
```

### 任务 7：实现 manifest、geoview 探针与六资产验证

**文件**：
- 创建：`internal/manifest/manifest.go`
- 创建：`internal/manifest/manifest_test.go`
- 创建：`internal/verify/geoview.go`
- 创建：`internal/verify/geoview_test.go`
- 创建：`internal/verify/assets.go`
- 创建：`internal/verify/assets_test.go`

**接口**：
- 消费：任务 2 的 sources/groups，任务 4 registry，任务 6 tools runner。
- 产出：`manifest.Build([]model.Source, []model.Group) manifest.Document`、`manifest.Write(string, manifest.Document) error`、`verify.TagLookup`（`Has(context.Context, model.Side, string) (bool, error)`）、`verify.NewProber(*tools.Runner, string, string) *verify.Prober`、`verify.Required(context.Context, verify.TagLookup, manifest.Document) error`、`verify.Forbidden(context.Context, verify.TagLookup, manifest.Document) error`、`verify.GroupRefs(context.Context, verify.TagLookup, []model.Group) error`、`verify.Assets(string, []string) error`、`verify.WriteSHA256(string) (string, error)`。

- [ ] **步骤 1：写失败测试**

`internal/manifest/manifest_test.go`：

```go
func TestManifestUsesOutputTagsAndForbidsLegacyTags(t *testing.T) {
	sources := []model.Source{{ID: "loyalsoldier-google", Outputs: model.Outputs{GeoSite: &model.Output{Tag: "google", Mode: model.MergeBase}}}}
	doc := manifest.Build(sources, nil)
	if !slices.Contains(doc.Required.GeoSite, "google") || slices.Contains(doc.Required.GeoSite, "loyalsoldier-google") {
		t.Fatalf("required=%v", doc.Required.GeoSite)
	}
	if !slices.Contains(doc.Forbidden.GeoSite, "loyalsoldier-google") { t.Fatalf("forbidden=%v", doc.Forbidden.GeoSite) }
}
```

`internal/verify/geoview_test.go` 用 fake geoview 写/不写临时 `.srs`，验证 required 非空、forbidden 缺失和 mixed-case `BilibiliHMT` 参数不转小写。`assets_test.go` 验证精确六项与 sha 内容 `hex + 两空格 + basename + newline`。

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/manifest ./internal/verify -v`

预期：FAIL，报缺少 manifest/verify 实现。

- [ ] **步骤 3：写最小实现**

Manifest JSON schema 固定为：

```go
type Tags struct { GeoSite []string `json:"geosite"`; GeoIP []string `json:"geoip"` }
type Target struct { Tag string `json:"tag"`; Mode model.Mode `json:"mode"` }
type SourceRecord struct { GeoSite *Target `json:"geosite,omitempty"`; GeoIP *Target `json:"geoip,omitempty"` }
type Document struct {
	SchemaVersion int `json:"schema_version"`
	Required Tags `json:"required"`
	Forbidden Tags `json:"forbidden"`
	Sources map[string]SourceRecord `json:"sources"`
}
```

Required = outputs + group refs + `geosite:cn/geoip:cn/geoip:private`，全部逐侧排序去重。Forbidden 明确包含旧 source 前缀 tag 和 `applications`，不从“另一 side 没声明”机械推导。

`NewProber` 绑定最终 geosite.dat 与 geoip.dat 路径；`Prober.Has(ctx, side, tag)` 按 side 选择 dat，为每个 tag 创建临时输出并调用：

```text
geoview -type <geosite|geoip> -action convert -input <dat> -list <tag> -output <tmp>/probe.srs -lowmem=true
```

只有命令成功且输出非空才算存在。`verify.GroupRefs` 对每个 group 引用调用 Has，错误同时包含 remarks 与 side:tag。

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./internal/manifest ./internal/verify -v`

预期：PASS；manifest 无旧 tag，BilibiliHMT 大小写传给 geoview，六资产集合严格。

- [ ] **步骤 5：提交**

```bash
git add internal/manifest internal/verify
git commit -m "feat(T7): 建立标签与发布验证门禁"
```

### 任务 8：实现 staging 与 build/publish 可恢复切换

**文件**：
- 创建：`internal/workspace/layout.go`
- 创建：`internal/workspace/transaction.go`
- 创建：`internal/workspace/transaction_test.go`

**接口**：
- 消费：标准文件系统。
- 产出：`workspace.Begin(string) (*Transaction, error)`、测试专用 `workspace.BeginWithFS(string, workspace.FS) (*Transaction, error)`、`(*Transaction).Layout() Layout`、`(*Transaction).Commit() error`、`(*Transaction).Abort() error`；`FS` 精确包含 `MkdirTemp/MkdirAll/Rename/RemoveAll/Stat`，`Layout` 暴露 staging `Build/Publish/DataMerged/IP/Manifest/GeoIPConfig` 路径。

- [ ] **步骤 1：写失败测试**

```go
func TestAbortAfterFinalProbePreservesOldBuildAndPublish(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "expected_tags.json"), "old-manifest")
	writeFile(t, filepath.Join(root, "publish", "geosite.dat"), "old-site")
	tx, err := workspace.Begin(root)
	if err != nil { t.Fatal(err) }
	writeFile(t, filepath.Join(tx.Layout().Publish, "geosite.dat"), "new-site")
	if err := tx.Abort(); err != nil { t.Fatal(err) }
	assertFile(t, filepath.Join(root, "build", "expected_tags.json"), "old-manifest")
	assertFile(t, filepath.Join(root, "publish", "geosite.dat"), "old-site")
}

func TestCommitSwitchesBothDirectoriesOrRollsBack(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "old"), "old")
	writeFile(t, filepath.Join(root, "publish", "old"), "old")
	tx, _ := workspace.Begin(root)
	writeFile(t, filepath.Join(tx.Layout().Build, "new"), "new")
	writeFile(t, filepath.Join(tx.Layout().Publish, "new"), "new")
	if err := tx.Commit(); err != nil { t.Fatal(err) }
	assertFile(t, filepath.Join(root, "build", "new"), "new")
	assertFile(t, filepath.Join(root, "publish", "new"), "new")
}
```

另用注入式 `FS.Rename` 在第二次 rename 失败，断言第一目录恢复。

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/workspace -v`

预期：FAIL，报缺少 Transaction。

- [ ] **步骤 3：写最小实现**

`FS` 固定签名，生产使用 `os` 适配器，故障测试包装该适配器并只覆盖 `Rename`：

```go
type FS interface {
	MkdirTemp(dir, pattern string) (string, error)
	MkdirAll(path string, perm fs.FileMode) error
	Rename(oldPath, newPath string) error
	RemoveAll(path string) error
	Stat(path string) (fs.FileInfo, error)
}
```

`Begin` 在 `<root>/.staging-<random>` 建 `build/`、`publish/`，所有 atomic file write 使用同目录 `CreateTemp → Write → Sync → Close → Rename`。`Commit` 对 build、publish 逐个执行 `final→backup`、`staged→final`，任何失败逆序恢复；成功删除 backup 和 staging。`Abort` 只删除 staging，不触碰 final。接口名明确称“可恢复切换”，不声称跨目录原子事务。

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./internal/workspace -v`

预期：PASS；成功、abort、第二次 rename 故障均保持定义的目录状态。

- [ ] **步骤 5：提交**

```bash
git add internal/workspace
git commit -m "feat(T8): 事务切换构建与发布目录"
```

## PassWall2 输出

### 任务 9：验证默认分流并渲染稳定 UCI

**文件**：
- 创建：`internal/passwall/render.go`
- 创建：`internal/passwall/render_test.go`
- 创建：`internal/passwall/testdata/groups.yaml`
- 创建：`internal/passwall/testdata/rules.golden`

**接口**：
- 消费：任务 2 的 `model.Group`，任务 7 的 group refs verifier。
- 产出：`passwall.ValidateGroups(context.Context, []model.Group, verify.TagLookup) error`（内部委托 `verify.GroupRefs`）、`passwall.Render([]model.Group) ([]byte, error)`。

- [ ] **步骤 1：写失败测试**

```go
func TestRenderAppleAndChinaWithStableOrder(t *testing.T) {
	groups := []model.Group{
		{ID: "apple_services", Remarks: "苹果服务", GeoSite: []string{"apple", "icloud"}},
		{ID: "china", Remarks: "中国大陆", GeoSite: []string{"cn"}, GeoIP: []string{"cn"}},
	}
	got, err := passwall.Render(groups)
	if err != nil { t.Fatal(err) }
	want, _ := os.ReadFile("testdata/rules.golden")
	if !bytes.Equal(got, want) { t.Fatalf("got:\n%s\nwant:\n%s", got, want) }
}

func TestMissingGroupTagNamesGroupAndReference(t *testing.T) {
	lookup := fakeLookup{missing: "geoip:not-exist"}
	err := passwall.ValidateGroups(context.Background(), []model.Group{{ID: "broken", Remarks: "坏组", GeoIP: []string{"not-exist"}}}, lookup)
	if err == nil || !strings.Contains(err.Error(), "坏组") || !strings.Contains(err.Error(), "geoip:not-exist") { t.Fatalf("err=%v", err) }
}

func TestYouTubePrecedesGoogle(t *testing.T) {
	groups, _ := config.LoadGroups("../../config/passwall2-groups.yaml")
	if index(groups, "youtube") >= index(groups, "google") { t.Fatalf("order=%v", groups) }
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/passwall -run 'Test(Render|Missing|YouTube)' -v`

预期：FAIL，报缺少 Render/ValidateGroups 或默认 groups 尚不存在。

- [ ] **步骤 3：写最小实现**

Renderer 固定每组输出：

```uci
config shunt_rules 'crs_apple_services'
	option remarks '苹果服务'
	option managed_by 'clash-rules-srs'
	option network 'tcp,udp'
	option domain_list 'geosite:apple
geosite:icloud'
```

ID = `crs_` + group ID，最长 64 字节；所有 UCI scalar 使用单引号并把 `'` 编码为 `'\''`，拒绝 CR/LF/NUL/C0/C1/U+2028/U+2029。组与 tag 顺序严格保留 YAML slice 顺序，不经 map range。

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./internal/passwall -run 'Test(Render|Missing|YouTube)' -v`

预期：PASS；golden 精确匹配，缺 tag 错误包含组和引用，YouTube 在 Google 前。

- [ ] **步骤 5：提交**

```bash
git add internal/passwall/render.go internal/passwall/render_test.go internal/passwall/testdata
git commit -m "feat(T9): 生成有序托管分流规则"
```

### 任务 10：生成事务安装脚本并覆盖迁移/回滚故障

**文件**：
- 创建：`internal/passwall/installer.go`
- 创建：`internal/passwall/install.sh.tmpl`
- 创建：`internal/passwall/installer_test.go`
- 创建：`internal/passwall/testdata/fake-uci.sh`
- 创建：`internal/passwall/testdata/fake-rule-update.lua`

**接口**：
- 消费：任务 9 的 UCI fragment，任务 7 的 dat SHA。
- 产出：`passwall.RenderInstaller(passwall.InstallOptions) ([]byte, error)`；`InstallOptions{Repo, ReleaseTag, GeoSiteSHA, GeoIPSHA string; Fragment []byte}`。

- [ ] **步骤 1：写失败测试**

`installer_test.go` 必须执行生成脚本，不只匹配文本：

```go
func TestInstallerIsIdempotentAndPreservesUserRulesAndNodes(t *testing.T) {
	h := newHarness(t)
	h.seedConfig(`config nodes 'node1'
	option remarks 'Keep Node'
config shunt_rules 'user_rule'
	option remarks 'Keep Rule'
config shunt_rules 'c2p_Proxy'
	option remarks 'Legacy'
config shunt_rules 'crs_old'
	option managed_by 'clash-rules-srs'
`)
	script := h.render(validInstallOptions(t))
	h.run(script); first := h.readConfig()
	h.run(script); second := h.readConfig()
	if first != second { t.Fatalf("not idempotent\nfirst=%s\nsecond=%s", first, second) }
	for _, want := range []string{"Keep Node", "Keep Rule", "crs_apple_services"} { if !strings.Contains(second, want) { t.Fatalf("missing %s", want) } }
	for _, gone := range []string{"c2p_Proxy", "crs_old"} { if strings.Contains(second, gone) { t.Fatalf("legacy remains: %s", gone) } }
}

func TestInstallerRollsBackWhenUpdaterReturnsSuccessWithWrongHash(t *testing.T) {
	h := newHarness(t)
	beforeConfig, beforeSite, beforeIP := h.snapshot()
	h.fakeUpdaterWrites("wrong-site", "wrong-ip", 0)
	err := h.runExpectError(h.render(validInstallOptions(t)))
	if err == nil { t.Fatal("expected hash failure") }
	h.assertSnapshot(beforeConfig, beforeSite, beforeIP)
}
```

再覆盖：UCI 有未提交修改、缺 `rule_update.lua`、stage commit 失败、live commit 失败、仅一侧旧 dat 存在、成功时先用不可变 URL 后持久化 latest、备份路径输出、`sh -n`、模板无固定 heredoc、fragment 仅以 base64 嵌入。

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/passwall -run 'TestInstaller' -v`

预期：FAIL，报缺少 RenderInstaller/harness template。

- [ ] **步骤 3：写最小实现**

`installer.go` 验证 repo 为 `^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`、release tag 为安全字符、SHA 为 64 hex，标准 base64 编码 fragment 后以 `text/template` 渲染 embedded `install.sh.tmpl`。模板必须完整实现以下状态机：

```sh
#!/bin/sh
set -eu
CONF="${PASSWALL2_CONF:-/etc/config/passwall2}"
UPDATER="${PASSWALL2_RULE_UPDATER:-/usr/share/passwall2/rule_update.lua}"
REPO='{{.Repo}}'
RELEASE_TAG='{{.ReleaseTag}}'
SITE_SHA='{{.GeoSiteSHA}}'
IP_SHA='{{.GeoIPSHA}}'
FRAGMENT_B64='{{.FragmentBase64}}'

command -v uci >/dev/null 2>&1 || { echo 'ERROR: uci not found' >&2; exit 2; }
command -v lua >/dev/null 2>&1 || { echo 'ERROR: lua not found' >&2; exit 2; }
command -v sha256sum >/dev/null 2>&1 || { echo 'ERROR: sha256sum not found' >&2; exit 2; }
command -v base64 >/dev/null 2>&1 || { echo 'ERROR: base64 not found' >&2; exit 2; }
[ -f "$CONF" ] || { echo "ERROR: missing $CONF" >&2; exit 2; }
[ -f "$UPDATER" ] || { echo "ERROR: missing $UPDATER" >&2; exit 2; }
[ -z "$(uci changes passwall2 2>/dev/null || true)" ] || { echo 'ERROR: uncommitted passwall2 changes' >&2; exit 2; }

ROOT=$(mktemp -d "${TMPDIR:-/tmp}/clash-rules-srs.XXXXXX")
STAGE="$ROOT/passwall2"
FRAGMENT="$ROOT/managed-rules"
BACKUP="$CONF.bak.$(date +%s).$$"
ASSET_DIR=$(uci -q get passwall2.@global_rules[0].v2ray_location_asset 2>/dev/null || echo /usr/share/v2ray/)
ASSET_DIR=${ASSET_DIR%/}
SITE="$ASSET_DIR/geosite.dat"
IP="$ASSET_DIR/geoip.dat"
cp "$CONF" "$BACKUP"
cp "$CONF" "$STAGE"
[ ! -f "$SITE" ] || cp "$SITE" "$ROOT/geosite.dat.old"
[ ! -f "$IP" ] || cp "$IP" "$ROOT/geoip.dat.old"
SUCCESS=0

restore() {
	cp "$BACKUP" "$CONF" || true
	uci -q commit passwall2 >/dev/null 2>&1 || true
	if [ -f "$ROOT/geosite.dat.old" ]; then cp "$ROOT/geosite.dat.old" "$SITE"; else rm -f "$SITE"; fi
	if [ -f "$ROOT/geoip.dat.old" ]; then cp "$ROOT/geoip.dat.old" "$IP"; else rm -f "$IP"; fi
}
cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	[ "$SUCCESS" -eq 1 ] || restore
	rm -rf "$ROOT"
	exit "$status"
}
trap cleanup EXIT HUP INT TERM

IMMUTABLE="https://github.com/$REPO/releases/download/$RELEASE_TAG"
LATEST="https://github.com/$REPO/releases/latest/download"
uci -c "$ROOT" -q set "passwall2.@global_rules[0].geosite_url=$IMMUTABLE/geosite.dat"
uci -c "$ROOT" -q set "passwall2.@global_rules[0].geoip_url=$IMMUTABLE/geoip.dat"

for ID in $(uci -c "$ROOT" -q show passwall2 | sed -n "s/^passwall2\.\([^.=]*\)\.managed_by='clash-rules-srs'$/\1/p"); do
	uci -c "$ROOT" -q delete "passwall2.$ID"
done
for ID in $(uci -c "$ROOT" -q show passwall2 | sed -n "s/^passwall2\.\(c2p_[A-Za-z0-9_]*\)=shunt_rules$/\1/p"); do
	uci -c "$ROOT" -q delete "passwall2.$ID"
done
uci -c "$ROOT" -q commit passwall2
printf '%s' "$FRAGMENT_B64" | base64 -d > "$FRAGMENT"
cat "$FRAGMENT" >> "$STAGE"
uci -c "$ROOT" -q show passwall2 >/dev/null
cp "$STAGE" "$CONF.new.$$"
chmod 600 "$CONF.new.$$"
mv "$CONF.new.$$" "$CONF"
uci -q commit passwall2

lua "$UPDATER" log 'geoip,geosite'
printf '%s  %s\n' "$SITE_SHA" "$SITE" | sha256sum -c -
printf '%s  %s\n' "$IP_SHA" "$IP" | sha256sum -c -
uci -q set "passwall2.@global_rules[0].geosite_url=$LATEST/geosite.dat"
uci -q set "passwall2.@global_rules[0].geoip_url=$LATEST/geoip.dat"
uci -q commit passwall2
SUCCESS=1
trap - EXIT HUP INT TERM
rm -rf "$ROOT"
echo '安装成功；用户分流优先于随后追加的托管分流。'
echo 'sing-box 需要 geoview >= 0.1.10。'
echo "备份: $BACKUP"
```

执行时若 fake UCI 证明 `uci -c "$ROOT"` 需要配置文件名固定为 `$ROOT/passwall2`，不得改变 staging layout。补充所有 shell 变量引用与 sed 输入的安全测试。

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./internal/passwall -run 'TestInstaller' -v`

预期：PASS；两次安装字节稳定，旧 c2p 清理，用户配置保留，假成功错误哈希完整回滚。

- [ ] **步骤 5：提交**

```bash
git add internal/passwall/installer.go internal/passwall/install.sh.tmpl internal/passwall/installer_test.go internal/passwall/testdata/fake-uci.sh internal/passwall/testdata/fake-rule-update.lua
git commit -m "feat(T10): 生成可回滚 PassWall2 安装器"
```

## 编排与仓库迁移

### 任务 11：编排完整 build/bootstrap/verify use-case 并接通 CLI

**文件**：
- 创建：`internal/app/deps.go`
- 创建：`internal/app/build.go`
- 创建：`internal/app/build_test.go`
- 创建：`internal/app/bootstrap.go`
- 创建：`internal/app/verify.go`
- 修改：`internal/cli/run.go`
- 修改：`internal/cli/run_test.go`
- 修改：`cmd/geodata-build/main.go`

**接口**：
- 消费：任务 2–10 的全部公开 internal 接口。
- 产出：`app.Commands() cli.Commands`、`app.Build(context.Context, app.BuildOptions, app.Dependencies) error`、`app.Bootstrap(context.Context, app.BootstrapOptions, tools.Executor) error`、`app.Verify(context.Context, app.VerifyOptions, *tools.Runner) error`；`BootstrapOptions{CacheRoot string}`、`VerifyOptions{Dat, Manifest string; Side model.Side; Forbid bool}`；正式 CLI flags 与 exit code。

- [ ] **步骤 1：写失败测试**

`internal/app/build_test.go` 使用 fake HTTP、fake tool runner/prober 和临时 community，覆盖完整调用顺序、source fetch 失败不发布、outputs 全部逐侧探针、Netflix 双侧标准 tag、最后 forbidden probe 失败保留旧目录：

```go
func TestBuildEmitsAndProbesEveryOutput(t *testing.T) {
	fx := newBuildFixture(t)
	fx.source("google", "payload:\n  - '+.google.test'\n")
	fx.source("netflix", "payload:\n  - DOMAIN-SUFFIX,netflix.test\n  - IP-CIDR,23.246.0.0/18\n")
	err := app.Build(context.Background(), fx.options(), fx.dependencies())
	if err != nil { t.Fatal(err) }
	for _, call := range []string{"geosite:google", "geosite:netflix", "geoip:netflix"} {
		if !slices.Contains(fx.prober.RequiredCalls, call) { t.Fatalf("missing probe %s in %v", call, fx.prober.RequiredCalls) }
	}
	manifest := readManifest(t, filepath.Join(fx.root, "build", "expected_tags.json"))
	if slices.Contains(manifest.Required.GeoSite, "loyalsoldier-google") { t.Fatalf("legacy tag in manifest") }
}

func TestBuildSourceFailureDoesNotSwitchPublish(t *testing.T) {
	fx := newBuildFixture(t)
	fx.seedPublished("old")
	fx.fetchError = errors.New("HTTP 404")
	err := app.Build(context.Background(), fx.options(), fx.dependencies())
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") { t.Fatalf("err=%v", err) }
	fx.assertPublished("old")
}

func TestBuildUnknownCustomTargetDoesNotSwitchPublish(t *testing.T) {
	fx := newBuildFixture(t)
	fx.seedPublished("old")
	fx.custom("geosite/googel.yaml", "payload:\n  - DOMAIN-SUFFIX,example.test\n")
	err := app.Build(context.Background(), fx.options(), fx.dependencies())
	if err == nil || !strings.Contains(err.Error(), "geosite:googel") { t.Fatalf("err=%v", err) }
	fx.assertPublished("old")
}

func TestBuildMissingGroupTagDoesNotPublishInstaller(t *testing.T) {
	fx := newBuildFixture(t)
	fx.seedPublished("old")
	fx.groups("groups:\n- id: broken\n  remarks: 坏组\n  geosite: []\n  geoip: [not-exist]\n")
	err := app.Build(context.Background(), fx.options(), fx.dependencies())
	if err == nil || !strings.Contains(err.Error(), "坏组") || !strings.Contains(err.Error(), "geoip:not-exist") { t.Fatalf("err=%v", err) }
	fx.assertPublished("old")
}

func TestBuildCreateCollisionDoesNotSwitchPublish(t *testing.T) {
	fx := newBuildFixture(t)
	fx.seedPublished("old")
	fx.baseTag(model.GeoSite, "collision")
	fx.output("source-a", model.GeoSite, model.Output{Tag: "collision", Mode: model.Create})
	err := app.Build(context.Background(), fx.options(), fx.dependencies())
	if err == nil || !strings.Contains(err.Error(), "source-a") || !strings.Contains(err.Error(), "collision") { t.Fatalf("err=%v", err) }
	fx.assertPublished("old")
}

func TestBuildForbiddenProbeFailurePreservesOldOutputs(t *testing.T) {
	fx := newBuildFixture(t)
	fx.seedPublished("old")
	fx.prober.FailOn = "geosite:loyalsoldier-google"
	err := app.Build(context.Background(), fx.options(), fx.dependencies())
	if err == nil || !strings.Contains(err.Error(), "loyalsoldier-google") { t.Fatalf("err=%v", err) }
	fx.assertBuildAndPublished("old")
}
```

`internal/cli/run_test.go` 增加 flag tests：build 缺 `--repo`/`--release-tag` 时 exit 2，未知 flag exit 2，verify side 非 geosite/geoip exit 2，bootstrap 默认 cache `.cache`。

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/app ./internal/cli ./cmd/geodata-build -v`

预期：FAIL，报缺少 app.Build/app.Commands 与新 flag parser。

- [ ] **步骤 3：写最小实现**

`BuildOptions` 固定字段：

```go
type BuildOptions struct {
	Root, Sources, Custom, Groups, Community, CacheRoot, Repo, ReleaseTag string
	SkipCompile bool
}
```

三个 `flag.FlagSet` 的公开契约固定如下；parse 错误、缺 required flag、额外 positional argument、非法 side 均返回 `&cli.UsageError{Err: err}`：

| 子命令 | flags 与默认值 | required |
|---|---|---|
| `bootstrap` | `--cache-root=.cache` | 无 |
| `build` | `--root=.`, `--sources=sources.yaml`, `--custom=custom`, `--groups=config/passwall2-groups.yaml`, `--community=.cache/upstream/domain-list-community/data`, `--cache-root=.cache`, `--skip-compile=false` | `--repo`、`--release-tag` |
| `verify` | `--forbid=false` | `--dat`、`--manifest`、`--side=geosite|geoip` |

所有相对路径在 flag parse 后相对 `--root` 解析并 `filepath.Clean`；`--community` 与 `--cache-root` 也遵守这一规则。`app.Commands()` 的三个 closure 分别调用上述 parser，再调用 `Bootstrap`、`Build`、`Verify`；只允许 `bootstrap|build|verify` 三个子命令，不读取环境变量覆盖这些 flags。

`deps.go` 定义精确的阶段边界；测试 fixture 注入函数，`ProductionDependencies` 的每个字段只调用任务 2–10 已定义的对应 package API：

```go
type buildState struct {
	Options       BuildOptions
	Tx            *workspace.Transaction
	Sources       []model.Source
	Groups        []model.Group
	Registry      *targets.Registry
	Contributions []model.Contribution
	BaseGeoIP     string
	GeoInputs     []geoip.Input
	Manifest      manifest.Document
	GeoSiteSHA    string
	GeoIPSHA      string
}

type buildStage func(context.Context, *buildState) error

type Dependencies struct {
	Begin            func(string) (*workspace.Transaction, error)
	LoadConfig       buildStage
	FetchBaseGeoIP   buildStage
	BuildRegistry    buildStage
	FetchRemote      buildStage
	LoadCustom       buildStage
	MergeGeoSite     buildStage
	PrepareGeoIP     buildStage
	WriteManifest    buildStage
	CompileGeoSite   buildStage
	CompileGeoIP     buildStage
	ProbeTags        buildStage
	ValidateGroups   buildStage
	RenderInstaller  buildStage
	VerifySixAssets  buildStage
}

type namedStage struct {
	name string
	run  buildStage
}

func Build(ctx context.Context, options BuildOptions, deps Dependencies) (err error) {
	tx, err := deps.Begin(options.Root)
	if err != nil { return fmt.Errorf("begin staging: %w", err) }
	state := &buildState{Options: options, Tx: tx}
	committed := false
	defer func() {
		if committed { return }
		if abortErr := tx.Abort(); abortErr != nil { err = errors.Join(err, fmt.Errorf("abort staging: %w", abortErr)) }
	}()

	prepare := []namedStage{
		{"load config", deps.LoadConfig},
		{"fetch base geoip", deps.FetchBaseGeoIP},
		{"validate target registry", deps.BuildRegistry},
		{"fetch and parse sources", deps.FetchRemote},
		{"load custom rules", deps.LoadCustom},
		{"merge geosite", deps.MergeGeoSite},
		{"prepare geoip", deps.PrepareGeoIP},
		{"write manifest", deps.WriteManifest},
	}
	for _, stage := range prepare {
		if stage.run == nil { return fmt.Errorf("%s: dependency is nil", stage.name) }
		if err := stage.run(ctx, state); err != nil { return fmt.Errorf("%s: %w", stage.name, err) }
	}
	if options.SkipCompile { return nil }

	finish := []namedStage{
		{"compile geosite", deps.CompileGeoSite},
		{"compile geoip", deps.CompileGeoIP},
		{"probe required and forbidden tags", deps.ProbeTags},
		{"validate passwall groups", deps.ValidateGroups},
		{"render installer and checksums", deps.RenderInstaller},
		{"verify six assets", deps.VerifySixAssets},
	}
	for _, stage := range finish {
		if stage.run == nil { return fmt.Errorf("%s: dependency is nil", stage.name) }
		if err := stage.run(ctx, state); err != nil { return fmt.Errorf("%s: %w", stage.name, err) }
	}
	if err := tx.Commit(); err != nil { return fmt.Errorf("commit build and publish: %w", err) }
	committed = true
	return nil
}
```

`ProductionDependencies` 的固定映射为：LoadConfig→`config.LoadSources/LoadGroups`；BuildRegistry→`targets.New`；LoadCustom→`rules.LoadCustom`；MergeGeoSite→`geosite.Merge`；PrepareGeoIP→`geoip.WriteInputs/WriteConfig`；WriteManifest→`manifest.Build/Write`；Compile 两阶段→`tools.Runner.Run`；ProbeTags→`verify.Required/Forbidden`；ValidateGroups→`passwall.ValidateGroups`；RenderInstaller→`passwall.Render/RenderInstaller` 与三次 `verify.WriteSHA256`；VerifySixAssets→`verify.Assets`。每个 closure 将中间值写入上述 `buildState` 的对应字段，不另建隐式全局状态。`--skip-compile` 完成 prepare 八阶段后返回，defer 删除 staging，不生成 installer、不切换 publish。

Compiler 参数固定：

```text
domain-list-custom --datapath=<data-merged> --datname=geosite.dat --outputpath=<publish> --exportlists= --togfwlist=
geoip convert -c <geoip-config.json>
```

CLI 使用独立 `flag.FlagSet`（`ContinueOnError`）解析每个子命令；usage/flag 错误返回 2，运行失败返回 1，成功返回 0。`cmd/geodata-build/main.go`：

```go
func main() { os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, app.Commands())) }
```

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./internal/app ./internal/cli ./cmd/geodata-build -v`

预期：PASS；fake build 完整通过，404/forbidden probe 不切换 publish，flags 返回稳定退出码。

- [ ] **步骤 5：提交**

```bash
git add internal/app internal/cli cmd/geodata-build
git commit -m "feat(T11): 编排全 Go geodata 构建"
```

### 任务 12：迁移 source、默认分流与 custom 模板

**文件**：
- 修改：`sources.yaml`
- 创建：`config/passwall2-groups.yaml`
- 创建：`custom/geosite/apple.yaml`
- 创建：`custom/geoip/cn.yaml`
- 创建：`internal/config/repository_contract_test.go`

**接口**：
- 消费：任务 2 的严格 loaders，任务 9 的顺序/引用 renderer。
- 产出：Spec 中 18 个 source output 契约、16 个默认托管组和两个语义空模板。

- [ ] **步骤 1：写失败测试**

`repository_contract_test.go` 载入真实仓库文件并精确比较：

```go
func TestRepositorySourcesMatchApprovedTargets(t *testing.T) {
	sources, err := config.LoadSources("../../sources.yaml")
	if err != nil { t.Fatal(err) }
	want := map[string]string{
		"loyalsoldier-reject": "geosite:reject:create",
		"loyalsoldier-icloud": "geosite:icloud:merge-base",
		"loyalsoldier-apple": "geosite:apple:merge-base",
		"loyalsoldier-google": "geosite:google:merge-base",
		"loyalsoldier-proxy": "geosite:proxy:create",
		"loyalsoldier-direct": "geosite:direct:create",
		"loyalsoldier-private": "geosite:private:merge-base",
		"loyalsoldier-gfw": "geosite:gfw:create",
		"loyalsoldier-tld-not-cn": "geosite:tld-not-cn:create",
		"loyalsoldier-telegramcidr": "geoip:telegram:merge-base",
		"loyalsoldier-cncidr": "geoip:cn:merge-base",
		"loyalsoldier-lancidr": "geoip:private:merge-base",
		"xiaolin-youtube": "geosite:youtube:merge-base",
		"xiaolin-netflix": "geosite:netflix:merge-base,geoip:netflix:merge-base",
		"xiaolin-spotify": "geosite:spotify:merge-base",
		"xiaolin-bilibili": "geosite:BilibiliHMT:create,geoip:BilibiliHMT:create",
		"xiaolin-tiktok": "geosite:tiktok:merge-base",
		"sukka-ai": "geosite:ai:create",
	}
	if diff := diffSources(sources, want); diff != "" { t.Fatal(diff) }
}

func TestDefaultTemplatesAreEmptyAndDocumentSupportedRules(t *testing.T) {
	for _, path := range []string{"../../custom/geosite/apple.yaml", "../../custom/geoip/cn.yaml"} {
		b, err := os.ReadFile(path); if err != nil { t.Fatal(err) }
		if !bytes.Contains(b, []byte("payload:")) || !bytes.Contains(b, []byte("DOMAIN")) && !bytes.Contains(b, []byte("IP-CIDR")) { t.Fatalf("missing docs in %s", path) }
	}
}

func TestDefaultTemplatesAreSemanticNoOps(t *testing.T) {
	cases := []struct {
		path string
		behavior model.Behavior
	}{
		{"../../custom/geosite/apple.yaml", model.Domain},
		{"../../custom/geoip/cn.yaml", model.IPCIDR},
	}
	for _, tc := range cases {
		f, err := os.Open(tc.path)
		if err != nil { t.Fatal(err) }
		buckets, parseErr := rules.Parse(f, model.YAML, tc.behavior)
		f.Close()
		if parseErr != nil { t.Fatal(parseErr) }
		if len(buckets.Domains) != 0 || len(buckets.CIDRs) != 0 { t.Fatalf("%s injects rules: %#v", tc.path, buckets) }
	}
}
```

另断言默认 groups 精确为 Spec 表中的 16 行、YouTube index < Google、Apple geosite 为 `[apple,icloud]`、China 双侧为 cn。

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/config -run 'TestRepository|TestDefault' -v`

预期：FAIL，旧 `sides` 被严格 loader 拒绝，groups/templates 不存在。

- [ ] **步骤 3：写最小实现**

把 `sources.yaml` 的每条 `name` 改为 `id`，删除 `sides`，逐侧写完整 outputs；URL/behavior/format 保持现值。示例：

```yaml
- id: xiaolin-netflix
  behavior: classical
  url: https://raw.githubusercontent.com/xiaolin-007/clash/main/rule/Netflix.txt
  outputs:
    geosite: {tag: netflix, mode: merge-base}
    geoip: {tag: netflix, mode: merge-base}
- id: xiaolin-bilibili
  behavior: classical
  url: https://raw.githubusercontent.com/xiaolin-007/clash/main/rule/BilibiliHMT.txt
  outputs:
    geosite: {tag: BilibiliHMT, mode: create}
    geoip: {tag: BilibiliHMT, mode: create}
```

`config/passwall2-groups.yaml` 写 Spec 的 16 行精确顺序，每组显式包含 `geosite` 和 `geoip`（空侧写 `[]`）。模板使用可直接取消注释的空 payload：

```yaml
payload:
  # geosite 支持：
  # - DOMAIN-SUFFIX,example.com
  # - DOMAIN,api.example.com
  # - DOMAIN-KEYWORD,example
  # - DOMAIN-REGEX,^.+\.example\.com$
```

geoip 模板对应列出 `IP-CIDR`、`IP-CIDR6` 与可选 `no-resolve`。注释项不得进入 payload。

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./internal/config ./internal/rules ./internal/passwall -v`

预期：PASS；18 个 source、16 个组、两个空模板全部通过严格 loader。

- [ ] **步骤 5：提交**

```bash
git add sources.yaml config/passwall2-groups.yaml custom internal/config/repository_contract_test.go
git commit -m "feat(T12): 迁移输出目标与默认分流"
```

### 任务 13：完成真实工具集成、旧 tag 负探针与全链路回归

**文件**：
- 创建：`internal/app/build_integration_test.go`
- 创建：`internal/app/testdata/community/google`
- 创建：`internal/app/testdata/community/youtube`
- 创建：`internal/app/testdata/community/bilibili`
- 创建：`internal/app/testdata/sources.yaml`
- 创建：`internal/app/testdata/groups.yaml`
- 创建：`internal/app/testdata/custom/geosite/BilibiliHMT.yaml`

**接口**：
- 消费：任务 11 完整 app、任务 12 repository config、`.cache/bin` 三工具。
- 产出：synthetic full-build integration harness；真实每日 build 前的离线回归门。

- [ ] **步骤 1：写失败测试**

`build_integration_test.go` 使用 `//go:build integration`，若 `.cache/bin` 缺工具则直接失败并提示先执行 bootstrap，不静默 skip。测试至少包含：

```go
func TestSyntheticFullBuildMergesAndSeparatesApprovedTags(t *testing.T) {
	root := newIntegrationRoot(t)
	err := app.Build(context.Background(), integrationOptions(root), realDependencies(root))
	if err != nil { t.Fatal(err) }
	doc := readManifest(t, filepath.Join(root, "build", "expected_tags.json"))
	for _, tag := range []string{"google", "youtube", "netflix", "BilibiliHMT"} {
		if !slices.Contains(doc.Required.GeoSite, tag) { t.Fatalf("missing geosite:%s", tag) }
	}
	for _, tag := range []string{"netflix", "BilibiliHMT"} {
		if !slices.Contains(doc.Required.GeoIP, tag) { t.Fatalf("missing geoip:%s", tag) }
	}
	for _, old := range []string{"loyalsoldier-google", "xiaolin-youtube", "xiaolin-bilibili"} {
		if !slices.Contains(doc.Forbidden.GeoSite, old) { t.Fatalf("not forbidden: %s", old) }
	}
	if !slices.Contains(doc.Forbidden.GeoIP, "xiaolin-bilibili") { t.Fatal("legacy geoip tag not forbidden") }
	assertMergedDomain(t, root, "google", "domain:googleapis.com")
	assertMergedDomain(t, root, "youtube", "full:youtubei.googleapis.com")
	assertDomainMatchesOnly(t, root, "hmt-only.example", "BilibiliHMT", "bilibili")
	assertGeoIPMatches(t, root, "netflix", "198.51.100.1")
	assertGeoIPMatches(t, root, "netflix", "198.51.100.200")
	assertGeoIPMatches(t, root, "BilibiliHMT", "203.0.113.1")
	assertGeoIPDoesNotMatch(t, root, "bilibili", "203.0.113.1")
	assertBefore(t, filepath.Join(root, "publish", "install_passwall2_rules.sh"), "YouTube", "Google 服务")
}
```

fixture 中 Google 含 `domain:googleapis.com`/`include:youtube`，YouTube 含 `full:youtubei.googleapis.com`，普通 bilibili 与 custom BilibiliHMT 含互不相同域名。GeoIP fixture 通过 pinned geoip 先生成含 `cn/private/netflix` 的本地 base dat，其中 netflix 为 `198.51.100.0/24`，再让 custom 合入同一 `/24` 与内部 `198.51.100.0/25`；测试分别探针 `.1` 与 `.200`，证明整个 `/24` 仍匹配。

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
go run ./cmd/geodata-build bootstrap --cache-root .cache
go test -tags=integration ./internal/app -run TestSyntheticFullBuild -v
```

预期：首次 FAIL 于尚未补齐的真实工具参数、fixture base 或 tag 语义断言；不能以 skip 代替红灯。

- [ ] **步骤 3：写最小实现**

补齐 integration fixture 构造器，只使用本地 `httptest.Server` 和临时文件，不依赖滚动远程 source。对生成的 `data-merged/google` / `youtube` / `BilibiliHMT` 文本做语义断言，再用真实 geoview 对最终 dat 做 required/forbidden 非空探针；GeoIP 重叠测试读取 converter 生成的 canonical list 证据或 geoview 转换输出，不在 wrapper 中重写 IPSet。

- [ ] **步骤 4：运行测试确认通过**

运行：

```bash
go test -tags=integration ./internal/app -run TestSyntheticFullBuild -v
go test ./...
go vet ./...
```

预期：integration PASS；普通测试和 vet 全绿；发布目录精确六资产。

- [ ] **步骤 5：提交**

```bash
git add internal/app/build_integration_test.go internal/app/testdata
git commit -m "test(T13): 验证真实 geodata 合并链路"
```

### 任务 14：删除 Python/Node/clash2passwall 并建立无旧运行时守卫

**文件**：
- 删除：`requirements.txt`
- 删除：`scripts/build.py`
- 删除：`scripts/probe_tags.py`
- 删除：`scripts/bootstrap_vendor.sh`
- 删除：`scripts/lib/**`
- 删除：`tests/**`
- 删除：`tools/clash2passwall/**`
- 修改：`.gitignore`
- 创建：`internal/app/runtime_contract_test.go`

**接口**：
- 消费：任务 11–13 已覆盖的 Go parity。
- 产出：仓库不再含旧生产/测试运行时；`.cache/` 取代 `vendor/`。

- [ ] **步骤 1：写失败测试**

创建 `internal/app/runtime_contract_test.go`：

```go
func TestRepositoryHasNoPythonOrNodeBuildRuntime(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{"requirements.txt", "scripts/build.py", "scripts/probe_tags.py", "scripts/bootstrap_vendor.sh", "tools/clash2passwall/package.json", "tools/clash2passwall/clash2passwall.js"} {
		if _, err := os.Stat(filepath.Join(root, path)); !errors.Is(err, os.ErrNotExist) { t.Errorf("legacy runtime remains: %s", path) }
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/app -run TestRepositoryHasNoPythonOrNodeBuildRuntime -v`

预期：FAIL，逐项列出仍存在的旧生产/测试运行时文件。

- [ ] **步骤 3：写最小实现**

在 Go parity 与 integration 已全绿后执行精确删除：

```bash
git rm requirements.txt scripts/build.py scripts/probe_tags.py scripts/bootstrap_vendor.sh
git rm -r scripts/lib tests tools/clash2passwall
```

`.gitignore` 删除 `vendor/`、`node_modules/` 相关项，新增：

```gitignore
.cache/
.staging-*/
```

不要删除 `.spec-dev/2026-07-30-geodata-selfhost/` 的历史 plan/acceptance，也不要清理用户未跟踪的 `dist/` 内容。

- [ ] **步骤 4：运行测试确认通过**

运行：`go test ./internal/app -run TestRepositoryHasNoPythonOrNodeBuildRuntime -v`

预期：PASS；旧生产和测试目录均不存在。然后运行 `go test ./...`，预期全绿。

- [ ] **步骤 5：提交**

```bash
git add .gitignore internal/app/runtime_contract_test.go
git add -u requirements.txt scripts tests tools/clash2passwall
git commit -m "refactor(T14): 移除 Python 与 Node 构建链"
```

### 任务 15：切换 CI、发布六资产并重写文档

**文件**：
- 修改：`.github/workflows/build.yml`
- 修改：`README.md`
- 修改：`context.md`
- 创建：`internal/app/workflow_contract_test.go`
- 创建：`internal/app/docs_contract_test.go`

**接口**：
- 消费：任务 11 CLI、任务 13 integration、任务 14 runtime guard。
- 产出：只读 build job → verified six-asset artifact → 独立 write publish job；Go-only 用户文档。

- [ ] **步骤 1：写失败测试**

`workflow_contract_test.go` 解析 workflow 文本并断言：

```go
func TestWorkflowBuildsAndPublishesExactSixAssets(t *testing.T) {
	b := readWorkflow(t)
	for _, required := range []string{
		"go test ./...", "go test -tags=integration ./internal/app", "go run ./cmd/geodata-build bootstrap",
		"go run ./cmd/geodata-build build", "install_passwall2_rules.sh", "install_passwall2_rules.sh.sha256sum",
		"persist-credentials: false", "permissions:\n      contents: read",
	} { if !bytes.Contains(b, []byte(required)) { t.Errorf("missing %q", required) } }
	for _, forbidden := range []string{"setup-python", "python ", "npm ", "node "} {
		if bytes.Contains(b, []byte(forbidden)) { t.Errorf("legacy command %q", forbidden) }
	}
}

func TestWorkflowHasNoLegacyRuntime(t *testing.T) {
	b := readWorkflow(t)
	for _, forbidden := range []string{"setup-python", "python ", "npm ", "node "} {
		if bytes.Contains(b, []byte(forbidden)) { t.Errorf("workflow contains %q", forbidden) }
	}
}
```

`docs_contract_test.go` 精确断言文档迁移：

```go
func TestDocsDescribeGoOnlyWorkflowAndManagedInstall(t *testing.T) {
	root := repositoryRoot(t)
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil { t.Fatal(err) }
		return b
	}
	for _, name := range []string{"README.md", "context.md"} {
		b := read(name)
		for _, required := range []string{
			"geodata-build bootstrap", "geodata-build build", "geodata-build verify",
			"custom/geosite", "custom/geoip", "config/passwall2-groups.yaml",
			"install_passwall2_rules.sh.sha256sum", "managed_by=clash-rules-srs", "回滚",
		} {
			if !bytes.Contains(b, []byte(required)) { t.Errorf("%s missing %q", name, required) }
		}
		for _, forbidden := range []string{"python scripts/", "npm --prefix", "clash2passwall.js"} {
			if bytes.Contains(b, []byte(forbidden)) { t.Errorf("%s contains legacy instruction %q", name, forbidden) }
		}
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

运行：`go test ./internal/app -run 'Test(Workflow|Docs)' -v`

预期：FAIL，旧 workflow 与文档仍引用 Python/Node、四资产与 clash2passwall。

- [ ] **步骤 3：写最小实现**

`.github/workflows/build.yml` 的 build job 顺序固定为：checkout（只读、不持久化凭据）→ setup-go 1.26.x → `go test ./...` → `go run ... bootstrap` → integration test → 计算 `TAG=geodata-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}` → `go run ... build --repo "$GITHUB_REPOSITORY" --release-tag "$TAG"` → 三份 `sha256sum -c` → 上传 `publish/`。publish job 下载 artifact，精确比较：

```text
geoip.dat
geoip.dat.sha256sum
geosite.dat
geosite.dat.sha256sum
install_passwall2_rules.sh
install_passwall2_rules.sh.sha256sum
```

publish job 创建与 build 注入完全相同的 tag，草稿上传六项、API 回读资产名与 target commit，再公开并切换 latest。所有 Actions 继续使用完整 SHA。

README/context 重写以下操作：

```bash
go run ./cmd/geodata-build bootstrap
go test ./...
go test -tags=integration ./internal/app
go run ./cmd/geodata-build build --repo OWNER/REPO --release-tag local-test
```

说明 custom YAML 支持类型、groups 顺序、默认分流映射、installer 校验命令与真实设备 deferred 边界。

- [ ] **步骤 4：运行测试确认通过**

运行：

```bash
go test ./...
go vet ./...
go test -tags=integration ./internal/app
```

预期：全部 PASS；runtime/workflow/docs contract 不含 Python/Node，integration 产出六资产。

- [ ] **步骤 5：提交**

```bash
git add .github/workflows/build.yml README.md context.md internal/app/workflow_contract_test.go internal/app/docs_contract_test.go
git commit -m "ci(T15): 切换 Go 构建与六资产发布"
```

## Spec Scenario 测试追踪

| Spec Scenario | 首个失败测试 / 验收行 |
|---|---|
| 扩展远程 source 创建的 BilibiliHMT | T13 `TestSyntheticFullBuildMergesAndSeparatesApprovedTags` |
| 拒绝拼错的本地目标 | T11 `TestBuildUnknownCustomTargetDoesNotSwitchPublish` |
| 未编辑模板不改变产物 | T12 `TestDefaultTemplatesAreSemanticNoOps` |
| 苹果服务包含声明的 tag | T9 `TestRenderAppleAndChinaWithStableOrder` |
| 缺失 tag 阻断脚本发布 | T11 `TestBuildMissingGroupTagDoesNotPublishInstaller` |
| YouTube 优先于 Google | T9 `TestYouTubePrecedesGoogle` |
| 中国大陆具备域名与 IP 规则 | T9 `TestRenderAppleAndChinaWithStableOrder` |
| 更新器成功退出但 dat 哈希错误 | T16 同名 e2e 验收行 |
| 完整成功后保留可恢复备份 | T16 同名 e2e 验收行 |
| 重复安装保持用户配置与托管组幂等 | T16 同名 e2e 验收行 |
| 首次安装清理旧转换器分流 | T16 同名 e2e 验收行 |
| Google 合并标准 tag | T13 `TestSyntheticFullBuildMergesAndSeparatesApprovedTags` |
| create 与 merge-base 前置条件严格执行 | T4 `TestCreateAndMergeBasePreconditions` |
| 对全部 outputs 逐侧探针 | T11 `TestBuildEmitsAndProbesEveryOutput` |
| Netflix 合入标准双侧 tag | T13 `TestSyntheticFullBuildMergesAndSeparatesApprovedTags` |
| 未声明的 community 碰撞仍失败 | T11 `TestBuildCreateCollisionDoesNotSwitchPublish` |
| 上游源失败不发布 | T11 `TestBuildSourceFailureDoesNotSwitchPublish` |
| Geosite 精确去重不扩大语义 | T4 `TestMergeDeduplicatesExactRulesButPreservesKindAndAttrs` |
| GeoIP 重叠 CIDR 规范化 | T13 `TestSyntheticFullBuildMergesAndSeparatesApprovedTags` |
| Google 父域不被破坏 | T13 `TestSyntheticFullBuildMergesAndSeparatesApprovedTags` |
| 港澳台规则不污染普通 bilibili | T13 `TestSyntheticFullBuildMergesAndSeparatesApprovedTags` |
| 草稿 Release 六资产回读 | T16 同名 release 验收行 |
| 干净环境完成全链路 | T16 同名 e2e 验收行 |
| 最后一个探针失败不破坏旧发布物 | T11 `TestBuildForbiddenProbeFailurePreservesOldOutputs` |

## 验收与收尾

### 任务 16：验收（acceptance-qa）

> 本任务由 executing-plans 收尾审查阶段触发 acceptance-qa 按下表执行，不参与逐任务连续执行；报告与证据落盘特性目录 `acceptance/` 子目录。

| Scenario / 检查项 | 维度 | 执行方式 | 目标 | 阈值/预期 | 验收证据 |
|---|---|---|---|---|---|
| 重复安装保持用户配置与托管组幂等 | e2e | 验收任务 (D) | fake-UCI harness | 两次执行后配置一致；用户规则/节点保留 | 前后配置、测试日志 |
| 首次安装清理旧转换器分流 | e2e | 验收任务 (D) | fake-UCI legacy fixture | `c2p_` 消失；非 c2p 用户 section 保留 | fixture diff |
| 更新器成功退出但 dat 哈希错误 | e2e | 验收任务 (D) | fake rule_update | 非零退出；配置与两个 dat 原字节恢复 | 故障注入日志与 hash |
| 完整成功后保留可恢复备份 | e2e | 验收任务 (D) | fake-UCI success path | latest URL、16 托管组、dat hash 生效；备份存在 | UCI dump、sha、备份路径 |
| 草稿 Release 六资产回读 | release | 验收任务 (D) | GitHub workflow dry/readback | 精确六项；三份 sha 校验成功 | API JSON 与 job log |
| 干净环境完成全链路 | e2e | 验收任务 (D) | ubuntu runner | 无 Python/Node；Go test/build/probe 全绿 | CI 完整日志 |
| PassWall2 真实设备更新 | operational | 验收任务 (D) | 可丢弃 OpenWrt/PassWall2 设备 | UCI、dat hash、xray/sing-box 查询均成功 | 设备日志；无设备则明确 DEFERRED |

### 任务 17：合并与清理

- [ ] **步骤 1：全量验证**

在 worktree 内运行：

```bash
go test ./...
go vet ./...
go test -tags=integration ./internal/app
git status --short
```

预期：测试/vet/integration 全绿；仅允许计划执行过程中明确记录的验收产物改动。失败必须修复后才进入合并。

- [ ] **步骤 2：合并回来源分支**

```bash
cd "$(dirname "$(git rev-parse --git-common-dir)")"
git merge plan/2026-07-31-go-geodata-pipeline
```

合并冲突、或主工作区有未提交改动 → 停下向计划作者确认，不强行合并。

- [ ] **步骤 3：清理**

```bash
git worktree remove .worktrees/go-geodata-pipeline
git branch -d plan/2026-07-31-go-geodata-pipeline
```

- [ ] **步骤 4：sync_commit 锚定**

```bash
SYNC=$(git rev-parse HEAD)
# 把 .spec-dev/2026-07-31-go-geodata-pipeline/spec/go-geodata-pipeline-design.md
# frontmatter 的 sync_commit: null 更新为上面的完整 SHA
git add .spec-dev/2026-07-31-go-geodata-pipeline/spec/go-geodata-pipeline-design.md
git commit -m "chore(spec): sync_commit 锚定 ${SYNC:0:7}"
```

此后 `git diff <sync_commit>..HEAD -- <covers glob>` 即本 Spec 上次确认同步以来的代码变化。任务 0 未建立本计划 worktree 时，只执行步骤 1 与步骤 4，步骤 2–3 交给原隔离机制。
