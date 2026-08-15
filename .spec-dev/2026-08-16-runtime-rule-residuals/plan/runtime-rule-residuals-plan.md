# 运行时规则残差优化 实施计划

> **执行方式**：使用 spec-dev 的 executing-plans skill 逐任务执行本计划；无该 skill 的环境直接从任务 0 起按序执行至最终任务。步骤用复选框（`- [ ]`）语法跟踪；脱离项目携带时连同特性目录（含 spec）整体带走。
>
> **偏差处理**：执行中发现计划与现实不符——小偏差（路径笔误、明显遗漏但意图清楚）就地修正并在提交信息中注明；接口、数据结构等契约级偏差停下向计划作者确认，不猜着改。

**目标**：从编译后的 GeoSite DAT 保守派生两个运行时残差标签，把默认托管组收敛为 14 个，并在不破坏兼容配置路由决策的前提下同步优化 PassWall2 与 Shadowrocket。

**Spec**：`.spec-dev/2026-08-16-runtime-rule-residuals/spec/runtime-rule-residuals-design.md`

**架构**：构建先把原始合并目录编译到 staging 中间 DAT，再经共享无损 decoder 推导 `crs-proxy-residual` 与 `crs-direct-residual`，最后第二次编译正式 DAT。PassWall2 安装器在 live UCI 切换前全量预检并迁移所有 Shunt 节点；Shadowrocket 仍从最终 DAT 展开，但 `tld_not_cn` 通过 group ID 单向引用 `proxy`。完整公共标签、reject 指纹和八资产闭集均保持兼容。

**技术栈**：Go 1.26、标准库 protobuf wire 解码、POSIX `/bin/sh` + `awk`、OpenWrt UCI、现有 `domain-list-custom`/`geoip`/`geoview` 工具链。

## 全局约束

- 不引入新的第三方依赖；继续使用 `go 1.26` 与现有固定工具链。
- 完整公共 `gfw`、`proxy`、`direct` 标签不得删除或被残差替换；`reject` 类型、值、attributes、顺序及第一组位置不得改变。
- generated-only GeoSite 标签固定为 `crs-proxy-residual` 与 `crs-direct-residual`；二者必须非空、禁止 source/community/custom 占用。
- 覆盖证明只允许单规则证明；attributes 规范化集合必须完全相同；不做多规则并集、正则语言包含或 GeoIP 证明。
- 默认组固定为 `reject → youtube → netflix → spotify → tiktok → ai → apple_services → telegram → google → proxy → tld_not_cn → private → cn → direct`。
- 删除 BilibiliHMT 来源、双侧标签、组、custom、夹具与文档；manifest 继续把 `xiaolin-bilibili`、`BilibiliHMT` 列为禁止回归项。
- PassWall2 迁移覆盖所有 `protocol='_shunt'` 节点及主字段、`_fakedns`、`_proxy_tag` 三族；冲突或 proxy/tld-not-cn 不等价时 live UCI 零修改。
- 单侧关闭迁移为非空一侧并启用合并全集是已批准行为变化，必须逐节点/字段输出 `WARNING`；不纳入路由等价承诺。
- `default_node`、reject、private、cn、direct 节点字段逐项保留；BilibiliHMT 与 gfw 废弃字段仅在预检全部通过后删除。
- Shadowrocket `tld_not_cn` 的唯一候选及 `policy-select-name` 都是按 ID 查得的 proxy remarks；不得硬编码中文 remarks。
- 八个 Release 资产闭集不变；本计划不增加性能报告发布资产。
- 设备路由等价只覆盖保留的 14 组及兼容 Shunt 配置；明确排除 BilibiliHMT 与 Xray `IPOnDemand`。
- 静态活动域名规则数、Shadowrocket `[Rule]` 行数和字节数分别不高于同源基线 70%；三种运行矩阵各自五次稳定 RSS 中位数不高于基线 95%。
- 提交信息使用中文，任务提交前缀使用 `feat(TN)`、`fix(TN)`、`test(TN)` 或 `docs(TN)`；未获得用户明确“提交/推送”授权时，执行器不得越过对应授权边界。

## 相关测试范围

本仓没有 affected-test 工具；按 spec `covers` 的 Go 影响面与真实编译夹具运行：

```bash
go test ./internal/...
go test -tags=integration ./internal/app
```

第二条命令需要 `.cache/bin/{domain-list-custom,geoip,geoview}`；缺失时先运行 `go run ./cmd/geodata-build bootstrap --cache-root .cache`，不得把“工具缺失”记为测试失败。

## 文件结构

### 新建

- `internal/geodat/decode.go`：共享的 GeoSite/GeoIP protobuf wire 解码；GeoSite 保留 regexp 与 bool-true attributes。
- `internal/geodat/decode_test.go`：类型、attributes、损坏 wire 与 GeoIP 回归夹具。
- `internal/geosite/residual.go`：纯函数覆盖证明、残差派生、确定排序和计数报告。
- `internal/geosite/residual_test.go`：proxy/direct 残差全部保守性 Scenario。
- `internal/geosite/generated.go`：只向空闲 tag 写 generated domain-list 文件。
- `internal/targets/generated.go`：两个 generated-only tag 的唯一注册表。

### 修改

- `internal/shadowrocket/expand.go`、`internal/shadowrocket/expand_test.go`：改用共享 decoder，并在 Shadowrocket 投影层跳过 regexp/attributes。
- `internal/targets/registry.go`、`internal/targets/registry_test.go`：在 source/base/custom 三条入口拒绝 generated-only tag。
- `internal/geosite/merge_test.go`：generated 文件写入、空残差及占用回归。
- `internal/workspace/layout.go`、`internal/workspace/transaction_test.go`：增加中间编译目录与 DAT 路径。
- `internal/app/build.go`、`internal/app/deps.go`、`internal/app/build_test.go`：加入中间编译、残差推导、最终编译和完整标签指纹校验阶段。
- `internal/shadowrocket/render.go`、`internal/shadowrocket/render_test.go`：按 ID 解析 `proxy → tld_not_cn` 单向策略引用。
- `internal/passwall/install.sh.tmpl`、`internal/passwall/installer_test.go`：Shunt 全局预检、暂存迁移、废弃字段清理与回滚断言。
- `sources.yaml`、`config/passwall2-groups.yaml`：删除 BilibiliHMT source/group，删除 gfw 组，切换 proxy/direct 残差引用。
- `internal/config/repository_contract_test.go`、`internal/rules/custom_test.go`、`internal/geosite/merge_test.go`、`internal/geoip/config_test.go`：更新 14 组、generated template 豁免与 BilibiliHMT 测试语义。
- `internal/manifest/manifest.go`、`internal/manifest/manifest_test.go`：残差 required 与 BilibiliHMT 双侧 forbidden。
- `internal/app/build_integration_test.go`、`internal/app/testdata/sources.yaml`、`internal/app/testdata/groups.yaml`、`internal/app/testdata/community/*`：同源两段编译、指纹、禁止标签和最终 conf 集成验证。
- `README.md`、`context.md`：更新 14 组、残差、迁移警告与 BilibiliHMT 破坏性删除说明。
- `.spec-dev/2026-08-15-shadowrocket-conf/spec/shadowrocket-conf-design.md`：交付完成时做部分取代回写。

### 删除

- `custom/geosite/BilibiliHMT.yaml`
- `custom/geoip/BilibiliHMT.yaml`
- `internal/app/testdata/custom/geosite/BilibiliHMT.yaml`

## Spec Scenario 映射

| Spec Scenario | 任务 | 首个失败测试或验收行 |
|---|---:|---|
| include 与属性在残差输入中已展开 | T1/T8 | `TestInclude与属性在残差输入中已展开` + 真实 compiler 子测试 |
| skip-compile 不生成残差发布物 | T4 | `TestSkipCompile不生成残差发布物` |
| GFW 与 proxy 合并且服务组优先 | T2 | `TestGFW与Proxy合并且服务组优先` |
| 不借用后序直连组裁剪 proxy | T2 | `Test不借用后序直连组裁剪Proxy` |
| tld-not-cn 与 cn 使 direct 规则不可达 | T2 | `TestTLDNotCN与CN使Direct规则不可达` |
| GeoIP 不作为域名覆盖证据 | T2 | `TestGeoIP不作为域名覆盖证据` |
| suffix 标签边界不产生假覆盖 | T2 | `TestSuffix标签边界不产生假覆盖` |
| 正则只做同类等值去重 | T2 | `Test正则只做同类等值去重` |
| 输出顺序可重现 | T2 | `Test输出顺序可重现` |
| 生成标签被来源占用 | T3/T8 | `Test生成标签被来源占用` + source/base 事务 Abort 子测试 |
| 中间 DAT 不污染最终探测缓存 | T4/T8 | `Test中间DAT不污染最终探测缓存` + 最终 prober 子测试 |
| 无法回写的 typed attribute 失败 | T1 | `Test无法回写的TypedAttribute失败` |
| 完整标签未被残差替换 | T4/T8 | `Test完整标签与Reject指纹门禁` + 集成指纹子测试 |
| reject 指纹不受优化影响 | T4/T8 | `Test完整标签与Reject指纹门禁` + reject 集成子测试 |
| proxy 缺失时继承 gfw | T6 | `TestProxy缺失时继承GFW` |
| 单侧关闭按非空一侧归一化 | T6 | `Test单侧关闭按非空一侧归一化` |
| gfw 与 proxy 冲突时零修改失败 | T6 | `TestGFW与Proxy冲突时零修改失败` |
| 重复安装保持幂等 | T6 | `TestInstallerIsIdempotentAndPreservesUserRulesAndNodes` |
| 两组均继承 Default | T6 | `Test两组均继承Default` |
| 等价出口条件不成立 | T6 | `Test等价出口条件不成立` |
| 后续步骤失败时恢复节点 | T6 | `Test后续步骤失败时恢复节点` |
| 成功安装后无废弃字段 | T6 | `Test成功安装后无废弃字段` |
| 默认代理和三组直连不被重写 | T6 | `Test默认代理和三组直连不被重写` |
| 默认 custom 模板审计忽略生成型标签 | T7 | `Test默认Custom模板审计忽略生成型标签` |
| 最终 DAT 不含 BilibiliHMT | T7/T8 | 仓库残留测试 + 最终双侧 prober 子测试 |
| Xray AsIs 域名路由等价 | T9 | e2e-router 验收表第一行 |
| Xray IPIfNonMatch 二次 IP 匹配等价 | T9 | e2e-router 验收表第二行 |
| sing-box 原生路由等价 | T9 | e2e-router 验收表第三行 |
| 静态规则规模下降至少 30% | T9 | perf-static 验收行 |
| 三种运行矩阵 RSS 下降至少 5% | T9 | perf-router 验收行 |
| 默认组 ID、顺序与标签精确匹配 | T7 | `Test默认组ID顺序与标签精确匹配` |
| 非中国域名跟随代理规则 | T5 | `Test非中国域名跟随代理规则` |
| remarks 变化时按 ID 建立引用 | T5 | `TestRemarks变化时按ID建立引用` |

---

## 隔离与核心数据层

### 任务 0：建立隔离工作区

- [x] **步骤 1：检测已有隔离**

运行：`git rev-parse --git-dir` 与 `git rev-parse --git-common-dir`
两者不同、且 `git rev-parse --show-superproject-working-tree` 无输出（排除 submodule）
→ 已在隔离工作区，跳过本任务。

- [x] **步骤 2：建立 worktree**

有原生 worktree 工具（如 EnterWorktree）或 using-git-worktrees skill 时优先使用（Codex 无原生 worktree 工具，直接走下面的手工路径）；否则手工降级：
确认 `.worktrees/` 已被忽略（`git check-ignore -q .worktrees`，当前仓库应命中 `.gitignore`），然后运行：

```bash
git worktree add .worktrees/runtime-rule-residuals -b plan/2026-08-16-runtime-rule-residuals
cd .worktrees/runtime-rule-residuals
```

- [x] **步骤 3：安装依赖并验证基线**

```bash
go mod download
go run ./cmd/geodata-build bootstrap --cache-root .cache
go test ./internal/...
go test -tags=integration ./internal/app
```

预期：两条 `go test` 均退出 0。基线失败 → 停下报告，先问再继续。

### 任务 1：共享无损 DAT decoder

**文件**：
- 创建：`internal/geodat/decode.go`
- 创建：`internal/geodat/decode_test.go`
- 修改：`internal/shadowrocket/expand.go`
- 修改：`internal/shadowrocket/expand_test.go`

**接口**：
- 消费：`model.DomainRule`、`model.DomainKind` 与 `netip.Prefix`。
- 产出：`func DecodeGeoSite([]byte) (map[string]SiteTag, error)`、`func DecodeGeoIP([]byte) (map[string]IPTag, error)`；map key 为小写 tag，record 的 `Name` 保留 DAT 原始大小写。

- [x] **步骤 1：写失败测试**

在 `internal/geodat/decode_test.go` 建立只依赖 `encoding/binary` 的 wire fixture helper，并写以下测试；每个 `t.Run` 名逐字沿用 spec Scenario：

```go
func TestInclude与属性在残差输入中已展开(t *testing.T) {
	data := siteDat(siteEntry("GOOGLE",
		domainEntry(0, "needle", attribute("cn", 2, 1)),
		domainEntry(1, "^regexp\\.example$"),
		domainEntry(2, "suffix.example"),
		domainEntry(3, "full.example"),
	))
	tags, err := geodat.DecodeGeoSite(data)
	if err != nil { t.Fatal(err) }
	want := []model.DomainRule{
		{Kind: model.DomainKeyword, Value: "needle", Attrs: []string{"@cn"}},
		{Kind: model.DomainRegexp, Value: "^regexp\\.example$"},
		{Kind: model.DomainSuffix, Value: "suffix.example"},
		{Kind: model.DomainFull, Value: "full.example"},
	}
	if got := tags["google"]; got.Name != "GOOGLE" || !reflect.DeepEqual(got.Domains, want) {
		t.Fatalf("tag=%#v want=%#v", got, want)
	}
}

func Test无法回写的TypedAttribute失败(t *testing.T) {
	tests := map[string][]byte{
		"false":   attribute("cn", 2, 0),
		"int":     attribute("cn", 3, 7),
		"unknown": append(attribute("cn", 2, 1), fieldVarint(9, 1)...),
	}
	for name, attr := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := geodat.DecodeGeoSite(siteDat(siteEntry("TEST", domainEntry(2, "bad.example", attr))))
			if err == nil || !strings.Contains(err.Error(), "TEST") ||
				!strings.Contains(err.Error(), "bad.example") || !strings.Contains(err.Error(), "attribute") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestGeoIPDecoder保持Shadowrocket前缀语义(t *testing.T) {
	want := []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("2001:db8::/32")}
	tags, err := geodat.DecodeGeoIP(ipDat(ipEntry("CN", cidrEntry(want[0]), cidrEntry(want[1]))))
	if err != nil { t.Fatal(err) }
	if got := tags["cn"].Prefixes; !reflect.DeepEqual(got, want) { t.Fatalf("got=%v want=%v", got, want) }
}
```

同一测试文件中的完整 wire fixture helper：

```go
func siteDat(entries ...[]byte) []byte {
	var result []byte
	for _, entry := range entries { result = append(result, fieldBytes(1, entry)...) }
	return result
}

func siteEntry(tag string, domains ...[]byte) []byte {
	result := fieldBytes(1, []byte(tag))
	for _, domain := range domains { result = append(result, fieldBytes(2, domain)...) }
	return result
}

func domainEntry(kind uint64, value string, attributes ...[]byte) []byte {
	result := fieldVarint(1, kind)
	result = append(result, fieldBytes(2, []byte(value))...)
	for _, attribute := range attributes { result = append(result, fieldBytes(3, attribute)...) }
	return result
}

func attribute(key string, typedField, typedValue uint64) []byte {
	result := fieldBytes(1, []byte(key))
	return append(result, fieldVarint(typedField, typedValue)...)
}

func ipDat(entries ...[]byte) []byte {
	var result []byte
	for _, entry := range entries { result = append(result, fieldBytes(1, entry)...) }
	return result
}

func ipEntry(tag string, cidrs ...[]byte) []byte {
	result := fieldBytes(1, []byte(tag))
	for _, cidr := range cidrs { result = append(result, fieldBytes(2, cidr)...) }
	return result
}

func cidrEntry(prefix netip.Prefix) []byte {
	result := fieldBytes(1, prefix.Addr().AsSlice())
	return append(result, fieldVarint(2, uint64(prefix.Bits()))...)
}

func fieldBytes(number uint64, value []byte) []byte {
	result := binary.AppendUvarint(nil, number<<3|2)
	result = binary.AppendUvarint(result, uint64(len(value)))
	return append(result, value...)
}

func fieldVarint(number, value uint64) []byte {
	result := binary.AppendUvarint(nil, number<<3)
	return binary.AppendUvarint(result, value)
}
```

保留并迁移原 `internal/shadowrocket/expand_test.go` 的损坏 DAT、重复 tag、未知普通字段、regexp 跳过和 include 展开测试。把 `TestRegexp被跳过` 的 attribute fixture 修成 compiler 的 bool-true 形态：

```go
typedAttribute := append(fieldBytes(1, []byte("cn")), fieldVarint(2, 1)...)
withAttribute := append(domainEntry(2, "kept.example"), fieldBytes(3, typedAttribute)...)
```

该测试已有 `len(got[0].Domains) == 1` 断言，继续证明带 attributes 的 suffix 与无 attributes 的同 `kind+value` 在 Shadowrocket 投影层只输出一条，且 regexp 不输出。

- [x] **步骤 2：运行测试确认失败**

运行：

```bash
go test ./internal/geodat ./internal/shadowrocket -run 'Test(Include与属性在残差输入中已展开|无法回写的TypedAttribute失败|GeoIPDecoder保持Shadowrocket前缀语义|Regexp被跳过)' -count=1
```

预期：FAIL，首先报 `package clash-rules-srs/internal/geodat is not in std` 或 `undefined: geodat.DecodeGeoSite`。

- [x] **步骤 3：写最小实现**

`internal/geodat/decode.go` 使用以下公开类型，并把现有 Shadowrocket GeoIP wire 逻辑搬入同一包；GeoSite decoder 不得在共享层跳过 regexp 或 attributes：

```go
package geodat

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"unicode"

	"clash-rules-srs/internal/model"
)

type SiteTag struct {
	Name    string
	Domains []model.DomainRule
}

type IPTag struct {
	Name     string
	Prefixes []netip.Prefix
}

func DecodeGeoSite(data []byte) (map[string]SiteTag, error) {
	result := make(map[string]SiteTag)
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil { return nil, err }
		if field != 1 { continue }
		if wire != 2 { return nil, fmt.Errorf("geosite entry has wire type %d", wire) }
		record, err := decodeSiteTag(payload)
		if err != nil { return nil, err }
		key := strings.ToLower(record.Name)
		if _, exists := result[key]; exists { return nil, fmt.Errorf("duplicate geosite tag %q", record.Name) }
		result[key] = record
	}
	return result, nil
}

func decodeSiteTag(data []byte) (SiteTag, error) {
	var record SiteTag
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil { return SiteTag{}, err }
		switch field {
		case 1:
			if wire != 2 { return SiteTag{}, fmt.Errorf("geosite tag has wire type %d", wire) }
			record.Name = string(payload)
		case 2:
			if wire != 2 { return SiteTag{}, fmt.Errorf("geosite domain has wire type %d", wire) }
			rule, err := decodeDomain(payload)
			if err != nil { return SiteTag{}, fmt.Errorf("tag %q: %w", record.Name, err) }
			record.Domains = append(record.Domains, rule)
		}
	}
	if record.Name == "" { return SiteTag{}, fmt.Errorf("geosite entry has empty tag") }
	return record, nil
}

func decodeDomain(data []byte) (model.DomainRule, error) {
	var kind uint64
	var value string
	var attrs []string
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil { return model.DomainRule{}, err }
		switch field {
		case 1:
			if wire != 0 { return model.DomainRule{}, fmt.Errorf("domain kind has wire type %d", wire) }
			kind = scalar
		case 2:
			if wire != 2 { return model.DomainRule{}, fmt.Errorf("domain value has wire type %d", wire) }
			value = string(payload)
		case 3:
			if wire != 2 { return model.DomainRule{}, fmt.Errorf("domain attribute has wire type %d", wire) }
			attr, err := decodeAttribute(payload)
			if err != nil { return model.DomainRule{}, fmt.Errorf("rule %q attribute: %w", value, err) }
			attrs = append(attrs, attr)
		}
	}
	if !validText(value) { return model.DomainRule{}, fmt.Errorf("invalid domain value %q", value) }
	var domainKind model.DomainKind
	switch kind {
	case 0: domainKind = model.DomainKeyword
	case 1: domainKind = model.DomainRegexp
	case 2: domainKind = model.DomainSuffix
	case 3: domainKind = model.DomainFull
	default: return model.DomainRule{}, fmt.Errorf("unsupported domain kind %d", kind)
	}
	sort.Strings(attrs)
	attrs = compactStrings(attrs)
	return model.DomainRule{Kind: domainKind, Value: value, Attrs: attrs}, nil
}

func decodeAttribute(data []byte) (string, error) {
	var key string
	var boolSeen, boolValue, typedSeen bool
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil { return "", err }
		switch field {
		case 1:
			if wire != 2 { return "", fmt.Errorf("key has wire type %d", wire) }
			key = string(payload)
		case 2:
			if wire != 0 || typedSeen { return "", fmt.Errorf("bool typed value is invalid") }
			boolSeen, boolValue, typedSeen = true, scalar != 0, true
		case 3:
			return "", fmt.Errorf("int typed value cannot be written losslessly")
		default:
			return "", fmt.Errorf("unknown attribute wire field %d", field)
		}
	}
	if key == "" || !validText(key) || strings.ContainsAny(key, "@, \\t") { return "", fmt.Errorf("invalid key %q", key) }
	if !boolSeen || !boolValue { return "", fmt.Errorf("bool typed value must be true") }
	return "@" + key, nil
}

func DecodeGeoIP(data []byte) (map[string]IPTag, error) {
	result := make(map[string]IPTag)
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil { return nil, err }
		if field != 1 { continue }
		if wire != 2 { return nil, fmt.Errorf("geoip entry has wire type %d", wire) }
		record, err := decodeIPTag(payload)
		if err != nil { return nil, err }
		key := strings.ToLower(record.Name)
		if _, exists := result[key]; exists { return nil, fmt.Errorf("duplicate geoip tag %q", record.Name) }
		result[key] = record
	}
	return result, nil
}

func decodeIPTag(data []byte) (IPTag, error) {
	var record IPTag
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil { return IPTag{}, err }
		switch field {
		case 1:
			if wire != 2 { return IPTag{}, fmt.Errorf("geoip tag has wire type %d", wire) }
			record.Name = string(payload)
		case 2:
			if wire != 2 { return IPTag{}, fmt.Errorf("geoip cidr has wire type %d", wire) }
			prefix, err := decodeCIDR(payload)
			if err != nil { return IPTag{}, fmt.Errorf("tag %q: %w", record.Name, err) }
			record.Prefixes = append(record.Prefixes, prefix)
		}
	}
	if record.Name == "" { return IPTag{}, fmt.Errorf("geoip entry has empty tag") }
	return record, nil
}

func decodeCIDR(data []byte) (netip.Prefix, error) {
	var addressBytes []byte
	var bits uint64
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil { return netip.Prefix{}, err }
		switch field {
		case 1:
			if wire != 2 { return netip.Prefix{}, fmt.Errorf("cidr address has wire type %d", wire) }
			addressBytes = append([]byte(nil), payload...)
		case 2:
			if wire != 0 { return netip.Prefix{}, fmt.Errorf("cidr prefix has wire type %d", wire) }
			bits = scalar
		}
	}
	address, ok := netip.AddrFromSlice(addressBytes)
	if !ok || address.Is4In6() { return netip.Prefix{}, fmt.Errorf("invalid cidr address %x", addressBytes) }
	if bits > uint64(address.BitLen()) { return netip.Prefix{}, fmt.Errorf("invalid /%d for %s", bits, address) }
	return netip.PrefixFrom(address, int(bits)).Masked(), nil
}

func compactStrings(values []string) []string {
	if len(values) < 2 { return values }
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] { result = append(result, value) }
	}
	return result
}

func validText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value { return false }
	for _, r := range value { if unicode.IsControl(r) { return false } }
	return true
}

type wireCursor struct{ data []byte }
func (c *wireCursor) more() bool { return len(c.data) != 0 }
func (c *wireCursor) next() (field, wire uint64, payload []byte, scalar uint64, err error) {
	key, n := binary.Uvarint(c.data)
	if n <= 0 { return 0, 0, nil, 0, fmt.Errorf("invalid protobuf field key") }
	c.data = c.data[n:]
	field, wire = key>>3, key&7
	if field == 0 { return 0, 0, nil, 0, fmt.Errorf("protobuf field number is zero") }
	switch wire {
	case 0:
		scalar, n = binary.Uvarint(c.data)
		if n <= 0 { return 0, 0, nil, 0, fmt.Errorf("invalid protobuf varint") }
		c.data = c.data[n:]
	case 1:
		if len(c.data) < 8 { return 0, 0, nil, 0, fmt.Errorf("truncated protobuf fixed64") }
		c.data = c.data[8:]
	case 2:
		length, size := binary.Uvarint(c.data)
		if size <= 0 { return 0, 0, nil, 0, fmt.Errorf("invalid protobuf byte length") }
		c.data = c.data[size:]
		if length > uint64(len(c.data)) { return 0, 0, nil, 0, fmt.Errorf("truncated protobuf bytes") }
		payload = c.data[:int(length)]
		c.data = c.data[int(length):]
	case 5:
		if len(c.data) < 4 { return 0, 0, nil, 0, fmt.Errorf("truncated protobuf fixed32") }
		c.data = c.data[4:]
	default:
		return 0, 0, nil, 0, fmt.Errorf("unsupported protobuf wire type %d", wire)
	}
	return field, wire, payload, scalar, nil
}
```

`internal/shadowrocket/expand.go` 删除私有 `siteRecord`、`ipRecord`、`decodeGeoSiteList`、`decodeGeoIPList` 与 `wireCursor`，改成：

```go
sites, err := geodat.DecodeGeoSite(siteBytes)
if err != nil { return nil, fmt.Errorf("decode geosite dat %s: %w", siteDat, err) }
ips, err := geodat.DecodeGeoIP(ipBytes)
if err != nil { return nil, fmt.Errorf("decode geoip dat %s: %w", ipDat, err) }
```

域名投影循环固定为：

```go
for _, rule := range record.Domains {
	if rule.Kind == model.DomainRegexp { continue }
	line := DomainLine{Kind: rule.Kind, Value: rule.Value}
	key := string(line.Kind) + "\x00" + line.Value
	if _, duplicate := domainSeen[key]; duplicate { continue }
	domainSeen[key] = struct{}{}
	expanded.Domains = append(expanded.Domains, line)
}
```

GeoIP 改读 `record.Prefixes`，其余公开 `Expand` 契约不变。

- [x] **步骤 4：运行测试确认通过**

```bash
gofmt -w internal/geodat internal/shadowrocket/expand.go internal/shadowrocket/expand_test.go
go test ./internal/geodat ./internal/shadowrocket -count=1
```

预期：PASS；现有 Shadowrocket golden 不变。

- [x] **步骤 5：提交**

```bash
git add internal/geodat internal/shadowrocket/expand.go internal/shadowrocket/expand_test.go
git commit -m "feat(T1): 共享无损 DAT 解码"
```

### 任务 2：保守残差推导器

**文件**：
- 创建：`internal/geosite/residual.go`
- 创建：`internal/geosite/residual_test.go`

**接口**：
- 消费：任务 1 的 `geodat.SiteTag` 与 `model.DomainRule`。
- 产出：`ResidualSpec`、`ResidualReport`、`Derive(map[string]geodat.SiteTag, ResidualSpec) ([]model.DomainRule, ResidualReport, error)`。

- [x] **步骤 1：写失败测试**

在 `internal/geosite/residual_test.go` 写一个 `siteTags(map[string][]model.DomainRule)` helper，并逐 Scenario 建立以下断言：

```go
func TestGFW与Proxy合并且服务组优先(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"gfw": {{Kind: model.DomainSuffix, Value: "shared.example"}},
		"proxy": {
			{Kind: model.DomainSuffix, Value: "shared.example"},
			{Kind: model.DomainFull, Value: "www.youtube.example"},
			{Kind: model.DomainSuffix, Value: "kept.example"},
		},
		"youtube": {{Kind: model.DomainSuffix, Value: "youtube.example"}},
	})
	got, report, err := geosite.Derive(tags, geosite.ResidualSpec{
		OutputTag: "crs-proxy-residual", BasisTags: []string{"gfw", "proxy"}, EarlierActiveTags: []string{"youtube"},
	})
	if err != nil { t.Fatal(err) }
	want := []model.DomainRule{{Kind: model.DomainSuffix, Value: "kept.example"}, {Kind: model.DomainSuffix, Value: "shared.example"}}
	if !reflect.DeepEqual(got, want) || report != (geosite.ResidualReport{Basis: 3, Kept: 2, Removed: 1}) {
		t.Fatalf("got=%#v report=%#v", got, report)
	}
}

func Test不借用后序直连组裁剪Proxy(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"proxy": {{Kind: model.DomainFull, Value: "only-cn.example"}},
		"cn": {{Kind: model.DomainSuffix, Value: "example"}},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{OutputTag: "crs-proxy-residual", BasisTags: []string{"proxy"}})
	if err != nil || len(got) != 1 || got[0].Value != "only-cn.example" { t.Fatalf("got=%#v err=%v", got, err) }
}

func TestTLDNotCN与CN使Direct规则不可达(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"direct": {
			{Kind: model.DomainFull, Value: "covered.foreign"},
			{Kind: model.DomainFull, Value: "covered.cn"},
			{Kind: model.DomainFull, Value: "kept.example"},
		},
		"tld-not-cn": {{Kind: model.DomainSuffix, Value: "foreign"}},
		"cn": {{Kind: model.DomainSuffix, Value: "cn"}},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{
		OutputTag: "crs-direct-residual", BasisTags: []string{"direct"}, EarlierActiveTags: []string{"tld-not-cn", "cn"},
	})
	if err != nil || len(got) != 1 || got[0].Value != "kept.example" { t.Fatalf("got=%#v err=%v", got, err) }
}

func TestGeoIP不作为域名覆盖证据(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{"direct": {{Kind: model.DomainFull, Value: "resolves-to-cn.example"}}})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{OutputTag: "crs-direct-residual", BasisTags: []string{"direct"}})
	if err != nil || len(got) != 1 { t.Fatalf("got=%#v err=%v", got, err) }
}

func TestSuffix标签边界不产生假覆盖(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"basis": {{Kind: model.DomainFull, Value: "example.com"}},
		"earlier": {{Kind: model.DomainSuffix, Value: "ample.com"}},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}, EarlierActiveTags: []string{"earlier"}})
	if err != nil || len(got) != 1 { t.Fatalf("got=%#v err=%v", got, err) }
}

func Test正则只做同类等值去重(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"basis": {
			{Kind: model.DomainRegexp, Value: "^x.+$", Attrs: []string{"@b", "@a"}},
			{Kind: model.DomainRegexp, Value: "^same$", Attrs: []string{"@a"}},
		},
		"earlier": {
			{Kind: model.DomainRegexp, Value: "^x.*$", Attrs: []string{"@a", "@b"}},
			{Kind: model.DomainRegexp, Value: "^same$", Attrs: []string{"@a"}},
		},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}, EarlierActiveTags: []string{"earlier"}})
	if err != nil || len(got) != 1 || got[0].Value != "^x.+$" { t.Fatalf("got=%#v err=%v", got, err) }
}

func Test输出顺序可重现(t *testing.T) {
	one := siteTags(map[string][]model.DomainRule{"basis": {
		{Kind: model.DomainSuffix, Value: "z.example", Attrs: []string{"@b", "@a"}},
		{Kind: model.DomainFull, Value: "a.example"},
	}})
	two := siteTags(map[string][]model.DomainRule{"basis": {
		{Kind: model.DomainFull, Value: "a.example"},
		{Kind: model.DomainSuffix, Value: "z.example", Attrs: []string{"@a", "@b"}},
	}})
	left, _, leftErr := geosite.Derive(one, geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}})
	right, _, rightErr := geosite.Derive(two, geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}})
	if leftErr != nil || rightErr != nil || !reflect.DeepEqual(left, right) { t.Fatalf("left=%#v right=%#v errors=%v/%v", left, right, leftErr, rightErr) }
}

func siteTags(input map[string][]model.DomainRule) map[string]geodat.SiteTag {
	result := make(map[string]geodat.SiteTag, len(input))
	for name, rules := range input {
		result[strings.ToLower(name)] = geodat.SiteTag{Name: name, Domains: append([]model.DomainRule(nil), rules...)}
	}
	return result
}
```

另加 table test 覆盖：full←full/suffix/keyword、suffix←suffix/keyword、keyword←keyword；attributes 不同一律保留；缺 basis tag 与空结果都返回含 output tag 的错误。

- [x] **步骤 2：运行测试确认失败**

```bash
go test ./internal/geosite -run 'Test(GFW与Proxy合并且服务组优先|不借用后序直连组裁剪Proxy|TLDNotCN与CN使Direct规则不可达|GeoIP不作为域名覆盖证据|Suffix标签边界不产生假覆盖|正则只做同类等值去重|输出顺序可重现)' -count=1
```

预期：FAIL，报 `undefined: geosite.Derive`。

- [x] **步骤 3：写最小实现**

`internal/geosite/residual.go`：

```go
package geosite

import (
	"fmt"
	"sort"
	"strings"

	"clash-rules-srs/internal/geodat"
	"clash-rules-srs/internal/model"
)

type ResidualSpec struct {
	OutputTag           string
	BasisTags           []string
	EarlierActiveTags   []string
	LaterEquivalentTags []string
}

type ResidualReport struct { Basis, Kept, Removed int }

func Derive(tags map[string]geodat.SiteTag, spec ResidualSpec) ([]model.DomainRule, ResidualReport, error) {
	if spec.OutputTag == "" || len(spec.BasisTags) == 0 { return nil, ResidualReport{}, fmt.Errorf("residual %q has no basis tags", spec.OutputTag) }
	basis, err := collect(tags, spec.BasisTags)
	if err != nil { return nil, ResidualReport{}, fmt.Errorf("residual %s basis: %w", spec.OutputTag, err) }
	coverageNames := append(append([]string(nil), spec.EarlierActiveTags...), spec.LaterEquivalentTags...)
	coverage, err := collect(tags, coverageNames)
	if err != nil { return nil, ResidualReport{}, fmt.Errorf("residual %s coverage: %w", spec.OutputTag, err) }
	kept := make([]model.DomainRule, 0, len(basis))
	for _, candidate := range basis {
		covered := false
		for _, existing := range coverage {
			if covers(existing, candidate) { covered = true; break }
		}
		if !covered { kept = append(kept, candidate) }
	}
	if len(kept) == 0 { return nil, ResidualReport{}, fmt.Errorf("residual %s is empty", spec.OutputTag) }
	report := ResidualReport{Basis: len(basis), Kept: len(kept), Removed: len(basis) - len(kept)}
	return kept, report, nil
}

func collect(tags map[string]geodat.SiteTag, names []string) ([]model.DomainRule, error) {
	byKey := make(map[string]model.DomainRule)
	for _, name := range names {
		record, exists := tags[strings.ToLower(name)]
		if !exists { return nil, fmt.Errorf("missing geosite:%s", name) }
		for _, rule := range record.Domains {
			normalized := normalize(rule)
			byKey[ruleKey(normalized)] = normalized
		}
	}
	result := make([]model.DomainRule, 0, len(byKey))
	for _, rule := range byKey { result = append(result, rule) }
	sort.Slice(result, func(i, j int) bool { return ruleKey(result[i]) < ruleKey(result[j]) })
	return result, nil
}

func normalize(rule model.DomainRule) model.DomainRule {
	rule.Attrs = append([]string(nil), rule.Attrs...)
	sort.Strings(rule.Attrs)
	if len(rule.Attrs) > 1 {
		out := rule.Attrs[:1]
		for _, attr := range rule.Attrs[1:] { if attr != out[len(out)-1] { out = append(out, attr) } }
		rule.Attrs = out
	}
	return rule
}

func ruleKey(rule model.DomainRule) string {
	return string(rule.Kind) + "\x00" + rule.Value + "\x00" + strings.Join(rule.Attrs, "\x00")
}

func covers(existing, candidate model.DomainRule) bool {
	if strings.Join(existing.Attrs, "\x00") != strings.Join(candidate.Attrs, "\x00") { return false }
	switch candidate.Kind {
	case model.DomainFull:
		switch existing.Kind {
		case model.DomainFull: return existing.Value == candidate.Value
		case model.DomainSuffix: return sameOrParent(existing.Value, candidate.Value)
		case model.DomainKeyword: return strings.Contains(candidate.Value, existing.Value)
		}
	case model.DomainSuffix:
		switch existing.Kind {
		case model.DomainSuffix: return sameOrParent(existing.Value, candidate.Value)
		case model.DomainKeyword: return strings.Contains(candidate.Value, existing.Value)
		}
	case model.DomainKeyword:
		return existing.Kind == model.DomainKeyword && strings.Contains(candidate.Value, existing.Value)
	case model.DomainRegexp:
		return existing.Kind == model.DomainRegexp && existing.Value == candidate.Value
	}
	return false
}

func sameOrParent(parent, value string) bool {
	return value == parent || strings.HasSuffix(value, "."+parent)
}
```

- [x] **步骤 4：运行测试确认通过**

```bash
gofmt -w internal/geosite/residual.go internal/geosite/residual_test.go
go test ./internal/geosite -count=1
```

预期：PASS；测试直接证明不存在 GeoIP 参数入口、后序直连组不会进入 proxy 的 coverage 集。

- [x] **步骤 5：提交**

```bash
git add internal/geosite/residual.go internal/geosite/residual_test.go
git commit -m "feat(T2): 实现保守残差推导"
```

### 任务 3：generated-only 目标与安全写入

**文件**：
- 创建：`internal/targets/generated.go`
- 创建：`internal/geosite/generated.go`
- 修改：`internal/targets/registry.go`
- 修改：`internal/targets/registry_test.go`
- 修改：`internal/geosite/merge_test.go`
- 修改：`internal/rules/custom_test.go`

**接口**：
- 消费：任务 2 输出的 `[]model.DomainRule`。
- 产出：`targets.ProxyResidualTag`、`targets.DirectResidualTag`、`targets.GeneratedGeoSiteTags()`、`targets.IsGenerated(model.Side, string)`、`geosite.WriteGenerated(string, string, []model.DomainRule) error`。

- [x] **步骤 1：写失败测试**

在 `internal/targets/registry_test.go` 增加：

```go
func Test生成标签被来源占用(t *testing.T) {
	for _, owner := range []string{"source", "community"} {
		t.Run(owner, func(t *testing.T) {
			lookup := func(side model.Side, tag string) (bool, error) {
				return owner == "community" && side == model.GeoSite && tag == targets.ProxyResidualTag, nil
			}
			var sources []model.Source
			if owner == "source" {
				output := model.Output{Tag: targets.ProxyResidualTag, Mode: model.Create}
				sources = []model.Source{{ID: "reserved-owner", Outputs: model.Outputs{GeoSite: &output}}}
			}
			_, err := targets.New(sources, lookup)
			if err == nil || !strings.Contains(err.Error(), targets.ProxyResidualTag) || !strings.Contains(err.Error(), "generated") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestGenerated标签拒绝CustomRequire(t *testing.T) {
	registry, err := targets.New(nil, func(model.Side, string) (bool, error) { return false, nil })
	if err != nil { t.Fatal(err) }
	err = registry.Require(model.GeoSite, targets.DirectResidualTag)
	if err == nil || !strings.Contains(err.Error(), "generated") { t.Fatalf("err=%v", err) }
}
```

在 `internal/geosite/merge_test.go` 增加：

```go
func TestGenerated标签安全写入(t *testing.T) {
	root := t.TempDir()
	rules := []model.DomainRule{
		{Kind: model.DomainSuffix, Value: "z.example", Attrs: []string{"@cn"}},
		{Kind: model.DomainFull, Value: "a.example"},
	}
	if err := geosite.WriteGenerated(root, targets.ProxyResidualTag, rules); err != nil { t.Fatal(err) }
	data, err := os.ReadFile(filepath.Join(root, targets.ProxyResidualTag))
	if err != nil { t.Fatal(err) }
	if string(data) != "domain:z.example @cn\nfull:a.example\n" { t.Fatalf("data=%q", data) }
	if err := geosite.WriteGenerated(root, targets.ProxyResidualTag, rules); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v", err)
	}
}

func TestGenerated标签拒绝空规则(t *testing.T) {
	err := geosite.WriteGenerated(t.TempDir(), targets.DirectResidualTag, nil)
	if err == nil || !strings.Contains(err.Error(), "empty") { t.Fatalf("err=%v", err) }
}
```

在 `internal/rules/custom_test.go` 增加真实 registry 路径：

```go
func TestGenerated标签拒绝Custom输入(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "geosite", targets.ProxyResidualTag+".yaml"), "payload:\n  - DOMAIN-SUFFIX,example.test\n")
	registry, err := targets.New(nil, func(model.Side, string) (bool, error) { return false, nil })
	if err != nil { t.Fatal(err) }
	got, err := rules.LoadCustom(root, registry)
	if err == nil || len(got) != 0 || !strings.Contains(err.Error(), targets.ProxyResidualTag) || !strings.Contains(err.Error(), "generated") {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}
```

- [x] **步骤 2：运行测试确认失败**

```bash
go test ./internal/targets ./internal/geosite ./internal/rules -run 'Test(生成标签被来源占用|Generated标签拒绝CustomRequire|Generated标签安全写入|Generated标签拒绝空规则)' -count=1
```

预期：FAIL，报 generated 常量或 `WriteGenerated` 未定义。

- [x] **步骤 3：写最小实现**

`internal/targets/generated.go`：

```go
package targets

import "clash-rules-srs/internal/model"

const (
	ProxyResidualTag  = "crs-proxy-residual"
	DirectResidualTag = "crs-direct-residual"
)

var generatedGeoSiteTags = []string{DirectResidualTag, ProxyResidualTag}

func GeneratedGeoSiteTags() []string {
	return append([]string(nil), generatedGeoSiteTags...)
}

func IsGenerated(side model.Side, tag string) bool {
	if side != model.GeoSite { return false }
	for _, generated := range generatedGeoSiteTags {
		if tag == generated { return true }
	}
	return false
}
```

在 `targets.New` 的 source output 合法性检查之前插入：

```go
if IsGenerated(item.side, item.output.Tag) {
	return nil, fmt.Errorf("source %q claims generated-only target %s:%s", source.ID, item.side, item.output.Tag)
}
```

在创建 `registry` 前检查 base/community：

```go
for _, tag := range GeneratedGeoSiteTags() {
	exists, err := lookup(model.GeoSite, tag)
	if err != nil { return nil, fmt.Errorf("lookup generated-only target geosite:%s: %w", tag, err) }
	if exists { return nil, fmt.Errorf("generated-only target geosite:%s already exists in base/community", tag) }
}
```

在 `Registry.Require` 初始化检查之后、`r.final` 查询之前插入：

```go
if IsGenerated(side, tag) {
	return fmt.Errorf("target %s:%s is generated-only and cannot accept custom input", side, tag)
}
```

`internal/geosite/generated.go`：

```go
package geosite

import (
	"fmt"
	"os"
	"path/filepath"

	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
)

func WriteGenerated(root, tag string, rules []model.DomainRule) error {
	if !targets.IsGenerated(model.GeoSite, tag) { return fmt.Errorf("geosite:%s is not a generated-only target", tag) }
	if len(rules) == 0 { return fmt.Errorf("generated geosite:%s is empty", tag) }
	path := filepath.Join(root, tag)
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("generated geosite:%s already exists", tag)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect generated geosite:%s: %w", tag, err)
	}
	if err := mergeTarget(path, rules); err != nil { return fmt.Errorf("write generated geosite:%s: %w", tag, err) }
	return nil
}
```

这里故意不允许第二次覆盖同名文件：同一次构建每个 generated tag 只能写一次；重复完整构建发生在新 staging 中，仍可幂等成功。

- [x] **步骤 4：运行测试确认通过**

```bash
gofmt -w internal/targets internal/geosite/generated.go internal/geosite/merge_test.go internal/rules/custom_test.go
go test ./internal/targets ./internal/geosite ./internal/rules -count=1
```

预期：PASS；source、community 与 custom 三条占用路径都在最终编译前失败。

- [x] **步骤 5：提交**

```bash
git add internal/targets internal/geosite/generated.go internal/geosite/merge_test.go internal/rules/custom_test.go
git commit -m "feat(T3): 保留生成型残差标签"
```

## 构建与平台渲染

### 任务 4：两段 GeoSite 编译与指纹门禁

**文件**：
- 修改：`internal/workspace/layout.go`
- 修改：`internal/workspace/transaction.go`
- 修改：`internal/workspace/transaction_test.go`
- 修改：`internal/app/build.go`
- 修改：`internal/app/deps.go`
- 修改：`internal/app/build_test.go`

**接口**：
- 消费：任务 1 的 `geodat.DecodeGeoSite`、任务 2 的 `geosite.Derive`、任务 3 的 generated tag 常量与 writer。
- 产出：`Layout.IntermediateGeoSite`；`Dependencies.CompileIntermediateGeoSite`、`Dependencies.DeriveResidualGeoSite`、`Dependencies.VerifyFullGeoSiteTags` 三个 stage；正式 `CompileGeoSite` 仍输出 `publish/geosite.dat`。

- [ ] **步骤 1：写失败测试**

在 `internal/workspace/transaction_test.go` 增加：

```go
func TestIntermediateGeoSite位于Build且不在Publish(t *testing.T) {
	tx, err := workspace.Begin(t.TempDir())
	if err != nil { t.Fatal(err) }
	defer tx.Abort()
	layout := tx.Layout()
	if filepath.Dir(layout.IntermediateGeoSite) != filepath.Join(layout.Build, "intermediate-geosite") {
		t.Fatalf("intermediate=%s build=%s", layout.IntermediateGeoSite, layout.Build)
	}
	if strings.HasPrefix(layout.IntermediateGeoSite, layout.Publish+string(os.PathSeparator)) { t.Fatal("intermediate DAT is under publish") }
}
```

在 `internal/app/build_test.go` 把 fixture dependencies 扩成新 stage，并新增：

```go
func TestSkipCompile不生成残差发布物(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	options := fixture.options()
	options.SkipCompile = true
	if err := Build(context.Background(), options, fixture.dependencies()); err != nil { t.Fatal(err) }
	fixture.assertBuildAndPublished("old")
	for _, forbidden := range []string{
		"compile intermediate geosite", "derive residual geosite", "compile final geosite",
		"verify full geosite tags", "render shadowrocket conf and checksum",
	} {
		if slices.Contains(fixture.calls, forbidden) { t.Fatalf("%s ran: %v", forbidden, fixture.calls) }
	}
}

func Test中间DAT不污染最终探测缓存(t *testing.T) {
	fixture := newBuildFixture(t)
	if err := Build(context.Background(), fixture.options(), fixture.dependencies()); err != nil { t.Fatal(err) }
	wantOrder := []string{
		"compile intermediate geosite", "derive residual geosite", "compile final geosite",
		"compile geoip", "verify full geosite tags", "validate passwall groups", "probe required and forbidden tags",
	}
	last := -1
	for _, name := range wantOrder {
		index := slices.Index(fixture.calls, name)
		if index <= last { t.Fatalf("stage %q out of order: %v", name, fixture.calls) }
		last = index
	}
}
```

再给 `verifyFullGeoSiteTags` 写 table test；`encodeAppTestSiteDAT` 是本测试文件内用 `binary.AppendUvarint` 实现的最小 encoder，按 `keyword=0/regexp=1/suffix=2/full=3` 写 Domain，并把每个 `@attr` 写为 key field 1 + bool field 2/value 1：

```go
func Test完整标签与Reject指纹门禁(t *testing.T) {
	baseline := map[string]geodat.SiteTag{}
	for _, tag := range []string{"gfw", "proxy", "direct", "reject"} {
		baseline[tag] = geodat.SiteTag{Name: strings.ToUpper(tag), Domains: []model.DomainRule{{
			Kind: model.DomainSuffix, Value: tag + ".example", Attrs: []string{"@cn"},
		}}}
	}
	baseline["reject"] = geodat.SiteTag{Name: "REJECT", Domains: []model.DomainRule{
		{Kind: model.DomainSuffix, Value: "ads-one.example", Attrs: []string{"@cn"}},
		{Kind: model.DomainFull, Value: "ads-two.example"},
	}}
	baseline[targets.ProxyResidualTag] = geodat.SiteTag{Name: targets.ProxyResidualTag, Domains: []model.DomainRule{{Kind: model.DomainFull, Value: "proxy-residual.example"}}}
	baseline[targets.DirectResidualTag] = geodat.SiteTag{Name: targets.DirectResidualTag, Domains: []model.DomainRule{{Kind: model.DomainFull, Value: "direct-residual.example"}}}

	writeFinal := func(tags map[string]geodat.SiteTag) string {
		path := filepath.Join(t.TempDir(), "geosite.dat")
		if err := os.WriteFile(path, encodeAppTestSiteDAT(tags), 0o600); err != nil { t.Fatal(err) }
		return path
	}
	if err := verifyFullGeoSiteTags(writeFinal(baseline), baseline); err != nil { t.Fatal(err) }

	for _, mutation := range []struct{ name, tag string; rule model.DomainRule }{
		{name: "kind", tag: "gfw", rule: model.DomainRule{Kind: model.DomainFull, Value: "gfw.example", Attrs: []string{"@cn"}}},
		{name: "value", tag: "proxy", rule: model.DomainRule{Kind: model.DomainSuffix, Value: "changed.example", Attrs: []string{"@cn"}}},
		{name: "attrs", tag: "direct", rule: model.DomainRule{Kind: model.DomainSuffix, Value: "direct.example", Attrs: []string{"@other"}}},
		{name: "order", tag: "reject"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			changed := cloneSiteTags(baseline)
			if mutation.name == "order" {
				rules := changed[mutation.tag].Domains
				changed[mutation.tag] = geodat.SiteTag{Name: "REJECT", Domains: []model.DomainRule{rules[1], rules[0]}}
			} else {
				changed[mutation.tag] = geodat.SiteTag{Name: strings.ToUpper(mutation.tag), Domains: []model.DomainRule{mutation.rule}}
			}
			err := verifyFullGeoSiteTags(writeFinal(changed), baseline)
			if err == nil || !strings.Contains(err.Error(), mutation.tag) { t.Fatalf("err=%v", err) }
		})
	}
}

func cloneSiteTags(input map[string]geodat.SiteTag) map[string]geodat.SiteTag {
	result := make(map[string]geodat.SiteTag, len(input))
	for key, tag := range input {
		tag.Domains = append([]model.DomainRule(nil), tag.Domains...)
		result[key] = tag
	}
	return result
}

func encodeAppTestSiteDAT(tags map[string]geodat.SiteTag) []byte {
	names := make([]string, 0, len(tags))
	for name := range tags { names = append(names, name) }
	sort.Strings(names)
	var document []byte
	for _, name := range names {
		tag := tags[name]
		entry := appTestFieldBytes(1, []byte(tag.Name))
		for _, rule := range tag.Domains {
			kind := map[model.DomainKind]uint64{
				model.DomainKeyword: 0, model.DomainRegexp: 1, model.DomainSuffix: 2, model.DomainFull: 3,
			}[rule.Kind]
			domain := appTestFieldVarint(1, kind)
			domain = append(domain, appTestFieldBytes(2, []byte(rule.Value))...)
			for _, attr := range rule.Attrs {
				attribute := appTestFieldBytes(1, []byte(strings.TrimPrefix(attr, "@")))
				attribute = append(attribute, appTestFieldVarint(2, 1)...)
				domain = append(domain, appTestFieldBytes(3, attribute)...)
			}
			entry = append(entry, appTestFieldBytes(2, domain)...)
		}
		document = append(document, appTestFieldBytes(1, entry)...)
	}
	return document
}

func appTestFieldBytes(number uint64, value []byte) []byte {
	result := binary.AppendUvarint(nil, number<<3|2)
	result = binary.AppendUvarint(result, uint64(len(value)))
	return append(result, value...)
}

func appTestFieldVarint(number, value uint64) []byte {
	result := binary.AppendUvarint(nil, number<<3)
	return binary.AppendUvarint(result, value)
}
```

另加 `missing` 子测试：从 clone 删除 `gfw` 后写 final，断言错误含 `gfw`。测试 encoder 不导入 production encoder，也不调用外部 compiler。

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/workspace ./internal/app -run 'Test(IntermediateGeoSite位于Build且不在Publish|SkipCompile不生成残差发布物|中间DAT不污染最终探测缓存|完整标签与Reject指纹门禁)' -count=1
```

预期：FAIL，报 `Layout.IntermediateGeoSite undefined` 或新 dependency stage 未定义。

- [ ] **步骤 3：写最小实现**

`internal/workspace/layout.go` 的 `Layout` 增加字段并在 `newLayout` 初始化：

```go
type Layout struct {
	Staging             string
	Build               string
	Publish             string
	DataMerged          string
	IP                  string
	Manifest            string
	GeoIPConfig         string
	BaseGeoIP           string
	IntermediateGeoSite string
}

// newLayout 返回值中：
IntermediateGeoSite: filepath.Join(build, "intermediate-geosite", "geosite.dat"),
```

`workspace.BeginWithFS` 的目录列表增加 `filepath.Dir(layout.IntermediateGeoSite)`。

`internal/app/build.go`：

```go
type buildState struct {
	Options              BuildOptions
	Tx                   *workspace.Transaction
	Sources              []model.Source
	Groups               []model.Group
	Registry             *targets.Registry
	Contributions        []model.Contribution
	BaseGeoIP            string
	GeoIPTemplate        *geoip.Template
	GeoInputs            []geoip.Input
	Manifest             manifest.Document
	GeoSiteSHA           string
	GeoIPSHA             string
	IntermediateSiteTags map[string]geodat.SiteTag
	ResidualReports      map[string]geosite.ResidualReport
	FinalLookup          verify.TagLookup
}

type Dependencies struct {
	Begin                      func(string) (*workspace.Transaction, error)
	LoadConfig                 buildStage
	FetchBaseGeoIP             buildStage
	BuildRegistry              buildStage
	FetchRemote                buildStage
	LoadCustom                 buildStage
	MergeGeoSite               buildStage
	PrepareGeoIP               buildStage
	WriteManifest              buildStage
	CompileIntermediateGeoSite buildStage
	DeriveResidualGeoSite      buildStage
	CompileGeoSite             buildStage
	CompileGeoIP               buildStage
	VerifyFullGeoSiteTags      buildStage
	ProbeTags                  buildStage
	ValidateGroups             buildStage
	RenderInstaller            buildStage
	RenderShadowrocket         buildStage
	VerifyReleaseAssets        buildStage
}
```

finish stages 必须是：

```go
finish := []namedStage{
	{"compile intermediate geosite", dependencies.CompileIntermediateGeoSite},
	{"derive residual geosite", dependencies.DeriveResidualGeoSite},
	{"compile final geosite", dependencies.CompileGeoSite},
	{"compile geoip", dependencies.CompileGeoIP},
	{"verify full geosite tags", dependencies.VerifyFullGeoSiteTags},
	{"validate passwall groups", dependencies.ValidateGroups},
	{"probe required and forbidden tags", dependencies.ProbeTags},
	{"render installer and checksums", dependencies.RenderInstaller},
	{"render shadowrocket conf and checksum", dependencies.RenderShadowrocket},
	{"verify release assets", dependencies.VerifyReleaseAssets},
}
```

`internal/app/deps.go` 增加共享 compiler helper：

```go
func compileGeoSite(ctx context.Context, runner *tools.Runner, root, dataPath, outputPath string) error {
	if runner == nil { return fmt.Errorf("tool runner is nil") }
	if err := os.MkdirAll(outputPath, 0o755); err != nil { return fmt.Errorf("create geosite output: %w", err) }
	if err := runner.Run(ctx, "domain-list-custom", root,
		"--datapath="+dataPath,
		"--datname=geosite.dat",
		"--outputpath="+outputPath,
		"--exportlists=",
		"--togfwlist=",
	); err != nil { return err }
	if err := os.Remove(filepath.Join(outputPath, "gfwlist.txt")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove compiler side asset: %w", err)
	}
	return nil
}
```

Production dependencies 的三个新 stage 与正式 compile：

```go
CompileIntermediateGeoSite: func(ctx context.Context, state *buildState) error {
	return compileGeoSite(ctx, runner, state.Options.Root, state.Tx.Layout().DataMerged, filepath.Dir(state.Tx.Layout().IntermediateGeoSite))
},
DeriveResidualGeoSite: func(_ context.Context, state *buildState) error {
	data, err := os.ReadFile(state.Tx.Layout().IntermediateGeoSite)
	if err != nil { return fmt.Errorf("read intermediate geosite: %w", err) }
	tags, err := geodat.DecodeGeoSite(data)
	if err != nil { return fmt.Errorf("decode intermediate geosite: %w", err) }
	state.IntermediateSiteTags = tags
	state.ResidualReports = make(map[string]geosite.ResidualReport, 2)
	proxySpec := geosite.ResidualSpec{
		OutputTag: targets.ProxyResidualTag,
		BasisTags: []string{"gfw", "proxy"},
		EarlierActiveTags: []string{"reject", "youtube", "netflix", "spotify", "tiktok", "ai", "apple", "icloud", "google"},
		LaterEquivalentTags: []string{"tld-not-cn"},
	}
	proxy, report, err := geosite.Derive(tags, proxySpec)
	if err != nil { return err }
	if err := geosite.WriteGenerated(state.Tx.Layout().DataMerged, proxySpec.OutputTag, proxy); err != nil { return err }
	state.ResidualReports[proxySpec.OutputTag] = report
	tags[strings.ToLower(proxySpec.OutputTag)] = geodat.SiteTag{Name: proxySpec.OutputTag, Domains: proxy}
	directSpec := geosite.ResidualSpec{
		OutputTag: targets.DirectResidualTag,
		BasisTags: []string{"direct"},
		EarlierActiveTags: []string{
			"reject", "youtube", "netflix", "spotify", "tiktok", "ai", "apple", "icloud", "google",
			targets.ProxyResidualTag, "tld-not-cn", "private", "cn",
		},
	}
	direct, report, err := geosite.Derive(tags, directSpec)
	if err != nil { return err }
	if err := geosite.WriteGenerated(state.Tx.Layout().DataMerged, directSpec.OutputTag, direct); err != nil { return err }
	state.ResidualReports[directSpec.OutputTag] = report
	return nil
},
CompileGeoSite: func(ctx context.Context, state *buildState) error {
	return compileGeoSite(ctx, runner, state.Options.Root, state.Tx.Layout().DataMerged, state.Tx.Layout().Publish)
},
VerifyFullGeoSiteTags: func(_ context.Context, state *buildState) error {
	return verifyFullGeoSiteTags(filepath.Join(state.Tx.Layout().Publish, "geosite.dat"), state.IntermediateSiteTags)
},
```

同文件增加门禁：

```go
func verifyFullGeoSiteTags(finalPath string, intermediate map[string]geodat.SiteTag) error {
	data, err := os.ReadFile(finalPath)
	if err != nil { return fmt.Errorf("read final geosite: %w", err) }
	final, err := geodat.DecodeGeoSite(data)
	if err != nil { return fmt.Errorf("decode final geosite: %w", err) }
	for _, tag := range []string{"gfw", "proxy", "direct", "reject"} {
		before, beforeOK := intermediate[tag]
		after, afterOK := final[tag]
		if !beforeOK || !afterOK || !reflect.DeepEqual(before.Domains, after.Domains) {
			return fmt.Errorf("full geosite:%s fingerprint changed", tag)
		}
	}
	for _, tag := range targets.GeneratedGeoSiteTags() {
		record, ok := final[tag]
		if !ok || len(record.Domains) == 0 { return fmt.Errorf("generated geosite:%s is missing or empty", tag) }
	}
	return nil
}
```

补充 imports：`reflect`、`strings`、`internal/geodat`。`finalProber` 不得在 `DeriveResidualGeoSite` 或 `VerifyFullGeoSiteTags` 中调用；它第一次出现仍是 final group/tag 校验，从而不会缓存中间 DAT。

更新 `buildFixture.dependencies()`：为四个新 stage 分别登记上述精确 name；中间 compile 写 `Layout.IntermediateGeoSite`，最终 compile 写 `publish/geosite.dat`，verify stage 在 fixture 中只记录顺序。原 `TestSkipCompile不产出Conf` 合并进新的 `TestSkipCompile不生成残差发布物`，避免两个 active Scenario 名竞争同一行为。

- [ ] **步骤 4：运行测试确认通过**

```bash
gofmt -w internal/workspace internal/app/build.go internal/app/deps.go internal/app/build_test.go
go test ./internal/workspace ./internal/app -count=1
```

预期：PASS；`--skip-compile` 在 prepare 后返回，中间/最终 compiler 与 publish switch 均未运行。

- [ ] **步骤 5：提交**

```bash
git add internal/workspace internal/app/build.go internal/app/deps.go internal/app/build_test.go
git commit -m "feat(T4): 增加两段 GeoSite 编译"
```

### 任务 5：Shadowrocket 代理组单向绑定

**文件**：
- 修改：`internal/shadowrocket/render.go`
- 修改：`internal/shadowrocket/render_test.go`

**接口**：
- 消费：现有 `[]shadowrocket.GroupRules`，不改变 YAML schema。
- 产出：`tld_not_cn` 按 `proxy` group ID 解析 remarks；`policy` 改为 `func policy(id, proxyRemarks string) ([]string, string, error)`。

- [ ] **步骤 1：写失败测试**

在 `internal/shadowrocket/render_test.go` 增加：

```go
func Test非中国域名跟随代理规则(t *testing.T) {
	rendered := mustRender(t, []shadowrocket.GroupRules{
		oneRule("proxy", "代理规则"),
		oneRule("tld_not_cn", "非中国域名"),
		oneRule("cn", "中国大陆"),
	}, "FlameMida/c2p-rules")
	want := "非中国域名 = select,代理规则,policy-select-name=代理规则\n"
	if !strings.Contains(rendered, want) { t.Fatalf("missing %q in\n%s", want, rendered) }
	if strings.Contains(rendered, "非中国域名 = select,PROXY,DIRECT") { t.Fatal("tld_not_cn remains independently selectable") }
}

func TestRemarks变化时按ID建立引用(t *testing.T) {
	rendered := mustRender(t, []shadowrocket.GroupRules{
		oneRule("tld_not_cn", "海外域名"),
		oneRule("proxy", "我的代理规则"),
		oneRule("cn", "中国大陆"),
	}, "FlameMida/c2p-rules")
	if !strings.Contains(rendered, "海外域名 = select,我的代理规则,policy-select-name=我的代理规则\n") {
		t.Fatalf("rendered=%s", rendered)
	}
	for name, groups := range map[string][]shadowrocket.GroupRules{
		"missing proxy": {oneRule("tld_not_cn", "海外域名"), oneRule("cn", "中国大陆")},
		"duplicate proxy id": {oneRule("proxy", "代理一"), oneRule("proxy", "代理二"), oneRule("tld_not_cn", "海外域名"), oneRule("cn", "中国大陆")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := shadowrocket.Render(groups, "FlameMida/c2p-rules")
			if err == nil || !strings.Contains(err.Error(), "proxy") { t.Fatalf("err=%v", err) }
		})
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/shadowrocket -run 'Test(非中国域名跟随代理规则|Remarks变化时按ID建立引用)' -count=1
```

预期：FAIL；旧 renderer 输出 `非中国域名 = select,PROXY,DIRECT,policy-select-name=PROXY`。

- [ ] **步骤 3：写最小实现**

在 `Render` 写任何 `[Proxy Group]` 行前完成 ID/remarks 两套索引：

```go
byID := make(map[string]GroupRules, len(groups))
seenRemarks := make(map[string]string, len(groups))
for _, rules := range groups {
	if _, duplicate := byID[rules.Group.ID]; duplicate {
		return nil, fmt.Errorf("duplicate group id %q", rules.Group.ID)
	}
	byID[rules.Group.ID] = rules
	if err := validateRemark(rules.Group.Remarks); err != nil { return nil, fmt.Errorf("group %q: %w", rules.Group.ID, err) }
	if reservedPolicyName(rules.Group.Remarks) { return nil, fmt.Errorf("group %q uses reserved policy group name %q", rules.Group.ID, rules.Group.Remarks) }
	if previousID, duplicate := seenRemarks[rules.Group.Remarks]; duplicate {
		return nil, fmt.Errorf("groups %q and %q repeat policy group name %q", previousID, rules.Group.ID, rules.Group.Remarks)
	}
	seenRemarks[rules.Group.Remarks] = rules.Group.ID
}
proxyRemarks := ""
if _, hasTLD := byID["tld_not_cn"]; hasTLD {
	proxy, ok := byID["proxy"]
	if !ok { return nil, fmt.Errorf("tld_not_cn requires proxy group") }
	proxyRemarks = proxy.Group.Remarks
}
```

原输出循环删除重复的 remark 验证，改为：

```go
candidates, selected, err := policy(rules.Group.ID, proxyRemarks)
if err != nil { return nil, err }
fmt.Fprintf(&output, "%s = select,%s,policy-select-name=%s\n",
	rules.Group.Remarks, strings.Join(candidates, ","), selected)
```

`policy` 完整替换为：

```go
func policy(id, proxyRemarks string) ([]string, string, error) {
	switch id {
	case "reject":
		return []string{"REJECT", "PROXY", "DIRECT"}, "REJECT", nil
	case "cn", "private", "direct":
		return []string{"DIRECT", "PROXY"}, "DIRECT", nil
	case "tld_not_cn":
		if proxyRemarks == "" { return nil, "", fmt.Errorf("tld_not_cn requires proxy remarks") }
		return []string{proxyRemarks}, proxyRemarks, nil
	default:
		return []string{"PROXY", "DIRECT"}, "PROXY", nil
	}
}
```

索引只形成 `tld_not_cn → proxy` 一条边；proxy 仍只含内建 `PROXY,DIRECT`，所以不存在回边或环。

- [ ] **步骤 4：运行测试确认通过**

```bash
gofmt -w internal/shadowrocket/render.go internal/shadowrocket/render_test.go
go test ./internal/shadowrocket -count=1
```

预期：PASS；原 reject/direct/default 与 rule tail golden 全部保持。

- [ ] **步骤 5：提交**

```bash
git add internal/shadowrocket/render.go internal/shadowrocket/render_test.go
git commit -m "feat(T5): 绑定非中国域名代理策略"
```

### 任务 6：PassWall2 Shunt 事务迁移

**文件**：
- 修改：`internal/passwall/install.sh.tmpl`
- 修改：`internal/passwall/installer_test.go`

**接口**：
- 消费：现有 standalone installer 的 staged UCI 文件 `$STAGE` 与完整 live backup `$BACKUP`。
- 产出：`preflight_shunt_nodes` 与 `migrate_and_filter_config` 两个 shell 函数；不新增外部命令依赖。

- [ ] **步骤 1：写失败测试**

在 `internal/passwall/installer_test.go` 增加统一 fixture：

```go
func compatibleShuntNode(id string) string {
	return fmt.Sprintf(`config nodes '%s'
	option protocol '_shunt'
	option remarks '兼容分流'
	option default_node 'proxy-node'
	option crs_gfw '_default'
	option crs_gfw_fakedns '1'
	option crs_gfw_proxy_tag 'proxy-tag'
	option crs_tld_not_cn '_default'
	option crs_tld_not_cn_fakedns '1'
	option crs_tld_not_cn_proxy_tag 'proxy-tag'
	option crs_bilibili_hmt '_direct'
	option crs_bilibili_hmt_fakedns '0'
	option crs_bilibili_hmt_proxy_tag 'old-bili-tag'
	option crs_reject '_blackhole'
	option crs_reject_fakedns '0'
	option crs_reject_proxy_tag 'reject-tag'
	option crs_private '_direct'
	option crs_private_fakedns '0'
	option crs_private_proxy_tag 'direct-tag'
	option crs_cn '_direct'
	option crs_cn_fakedns '0'
	option crs_cn_proxy_tag 'direct-tag'
	option crs_direct '_direct'
	option crs_direct_fakedns '0'
	option crs_direct_proxy_tag 'direct-tag'
`, id)
}
```

随后逐 Scenario 写真实 `/bin/sh` 集成测试：

```go
func TestProxy缺失时继承GFW(t *testing.T) {
	h := newHarness(t)
	h.seedConfig(compatibleShuntNode("shunt_a"))
	h.run(h.render(validInstallOptions()), true)
	got := h.readConfig()
	for _, want := range []string{"option crs_proxy '_default'", "option crs_proxy_fakedns '1'", "option crs_proxy_proxy_tag 'proxy-tag'"} {
		if !strings.Contains(got, want) { t.Fatalf("missing %q in\n%s", want, got) }
	}
}

func Test单侧关闭按非空一侧归一化(t *testing.T) {
	h := newHarness(t)
	node := strings.ReplaceAll(compatibleShuntNode("shunt_a"), "\toption crs_gfw '_default'\n", "")
	node = strings.ReplaceAll(node, "\toption crs_gfw_fakedns '1'\n", "")
	node = strings.ReplaceAll(node, "\toption crs_gfw_proxy_tag 'proxy-tag'\n", "")
	node = strings.Replace(node, "\toption crs_tld_not_cn '_default'\n", "\toption crs_proxy '_default'\n\toption crs_tld_not_cn '_default'\n", 1)
	node = strings.Replace(node, "\toption crs_tld_not_cn_fakedns '1'\n", "\toption crs_proxy_fakedns '1'\n\toption crs_tld_not_cn_fakedns '1'\n", 1)
	node = strings.Replace(node, "\toption crs_tld_not_cn_proxy_tag 'proxy-tag'\n", "\toption crs_proxy_proxy_tag 'proxy-tag'\n\toption crs_tld_not_cn_proxy_tag 'proxy-tag'\n", 1)
	h.seedConfig(node)
	output := h.run(h.render(validInstallOptions()), true)
	if !strings.Contains(output, "WARNING: Shunt node shunt_a") || !strings.Contains(output, "merged universe enabled") {
		t.Fatalf("missing warning:\n%s", output)
	}
}

func TestGFW与Proxy冲突时零修改失败(t *testing.T) {
	h := newHarness(t)
	node := strings.Replace(compatibleShuntNode("shunt_a"), "\toption crs_tld_not_cn '_default'\n", "\toption crs_proxy 'other-node'\n\toption crs_tld_not_cn '_default'\n", 1)
	h.seedConfig(node)
	beforeConfig, beforeSite, beforeIP := h.snapshot()
	output := h.run(h.render(validInstallOptions()), false)
	if !strings.Contains(output, "shunt_a") || !strings.Contains(output, "crs_gfw/crs_proxy") { t.Fatalf("output=%s", output) }
	h.assertSnapshot(beforeConfig, beforeSite, beforeIP)
}

func Test两组均继承Default(t *testing.T) {
	h := newHarness(t)
	h.seedConfig(compatibleShuntNode("shunt_a"))
	h.run(h.render(validInstallOptions()), true)
}

func Test等价出口条件不成立(t *testing.T) {
	for name, mutate := range map[string]func(string) string{
		"main": func(node string) string {
			return strings.Replace(node, "\toption crs_gfw '_default'\n", "\toption crs_proxy 'other-node'\n", 1)
		},
		"fakedns": func(node string) string {
			node = strings.Replace(node, "\toption crs_gfw '_default'\n", "\toption crs_proxy '_default'\n", 1)
			node = strings.Replace(node, "\toption crs_gfw_fakedns '1'\n", "\toption crs_proxy_fakedns '0'\n", 1)
			return strings.Replace(node, "\toption crs_gfw_proxy_tag 'proxy-tag'\n", "\toption crs_proxy_proxy_tag 'proxy-tag'\n", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			node := mutate(compatibleShuntNode("shunt_a"))
			h.seedConfig(node)
			beforeConfig, beforeSite, beforeIP := h.snapshot()
			output := h.run(h.render(validInstallOptions()), false)
			if !strings.Contains(output, "proxy/tld_not_cn mismatch") { t.Fatalf("output=%s", output) }
			h.assertSnapshot(beforeConfig, beforeSite, beforeIP)
		})
	}
}
```

扩展现有幂等测试，让 extra config 包含 `compatibleShuntNode("shunt_a")`；其已有 first/second 全文比较直接覆盖“重复安装保持幂等”。再新增：

```go
func Test后续步骤失败时恢复节点(t *testing.T) {
	h := newHarness(t)
	h.seedConfig(compatibleShuntNode("shunt_a"))
	beforeConfig, beforeSite, beforeIP := h.snapshot()
	h.siteContent = "wrong-site"
	h.ipContent = "wrong-ip"
	h.run(h.render(validInstallOptions()), false)
	h.assertSnapshot(beforeConfig, beforeSite, beforeIP)
}

func Test成功安装后无废弃字段(t *testing.T) {
	h := newHarness(t)
	h.seedConfig(compatibleShuntNode("shunt_a") + "\n" + compatibleShuntNode("shunt_b"))
	h.run(h.render(validInstallOptions()), true)
	got := h.readConfig()
	for _, field := range []string{
		"crs_gfw", "crs_gfw_fakedns", "crs_gfw_proxy_tag",
		"crs_bilibili_hmt", "crs_bilibili_hmt_fakedns", "crs_bilibili_hmt_proxy_tag",
	} {
		if strings.Contains(got, "option "+field+" ") { t.Fatalf("%s remains in\n%s", field, got) }
	}
}

func Test默认代理和三组直连不被重写(t *testing.T) {
	h := newHarness(t)
	h.seedConfig(compatibleShuntNode("shunt_a"))
	keep := []string{
		"option default_node 'proxy-node'",
		"option crs_reject '_blackhole'", "option crs_reject_fakedns '0'", "option crs_reject_proxy_tag 'reject-tag'",
		"option crs_private '_direct'", "option crs_private_fakedns '0'", "option crs_private_proxy_tag 'direct-tag'",
		"option crs_cn '_direct'", "option crs_cn_fakedns '0'", "option crs_cn_proxy_tag 'direct-tag'",
		"option crs_direct '_direct'", "option crs_direct_fakedns '0'", "option crs_direct_proxy_tag 'direct-tag'",
	}
	h.run(h.render(validInstallOptions()), true)
	got := h.readConfig()
	for _, line := range keep {
		if strings.Count(got, line) != 1 { t.Fatalf("line %q changed in\n%s", line, got) }
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/passwall -run 'Test(Proxy缺失时继承GFW|单侧关闭按非空一侧归一化|GFW与Proxy冲突时零修改失败|两组均继承Default|等价出口条件不成立|后续步骤失败时恢复节点|成功安装后无废弃字段|默认代理和三组直连不被重写|InstallerIsIdempotentAndPreservesUserRulesAndNodes)' -count=1
```

预期：FAIL；旧 installer 不迁移 `crs_gfw*`、不检查 proxy/tld-not-cn，且不会清理 BilibiliHMT options。

- [ ] **步骤 3：写最小实现**

在 `install.sh.tmpl` 的 `trap` 定义之后、phase 3 前增加两个函数。预检读取 `$STAGE`，扫描完全部节点后才以非零退出；它只读 staging，live UCI 尚未变化：

```sh
preflight_shunt_nodes() {
	awk '
		function unquote(value) {
			if (length(value) >= 2 && substr(value, 1, 1) == "\047" && substr(value, length(value), 1) == "\047")
				return substr(value, 2, length(value) - 2)
			return value
		}
		function reset( key) {
			for (key in values) delete values[key]
			section = ""; id = ""; protocol = ""
		}
		function check_family(suffix, label, gfw, proxy, tld, merged) {
			gfw = values["crs_gfw" suffix]
			proxy = values["crs_proxy" suffix]
			tld = values["crs_tld_not_cn" suffix]
			if (gfw != "" && proxy != "" && gfw != proxy) {
				printf "ERROR: Shunt node %s conflict in %s: crs_gfw/crs_proxy differ\n", id, label > "/dev/stderr"
				failed = 1
				return
			}
			merged = proxy != "" ? proxy : gfw
			if (merged != tld) {
				printf "ERROR: Shunt node %s proxy/tld_not_cn mismatch in %s\n", id, label > "/dev/stderr"
				failed = 1
			}
			if ((gfw == "") != (proxy == ""))
				printf "WARNING: Shunt node %s %s normalized from one-sided close; merged universe enabled\n", id, label > "/dev/stderr"
		}
		function check_block() {
			if (section != "nodes" || protocol != "_shunt") return
			check_family("", "main")
			check_family("_fakedns", "fakedns")
			check_family("_proxy_tag", "proxy_tag")
		}
		$1 == "config" {
			check_block(); reset()
			section = unquote($2); id = unquote($3)
			next
		}
		$1 == "option" {
			name = unquote($2)
			raw = $0
			sub(/^[ \t]*option[ \t]+[^ \t]+[ \t]+/, "", raw)
			values[name] = unquote(raw)
			if (name == "protocol") protocol = values[name]
		}
		END { check_block(); exit failed ? 1 : 0 }
	' "$STAGE"
}

migrate_and_filter_config() {
	awk '
		function unquote(value) {
			if (length(value) >= 2 && substr(value, 1, 1) == "\047" && substr(value, length(value), 1) == "\047")
				return substr(value, 2, length(value) - 2)
			return value
		}
		function reset( key) {
			for (key in values) delete values[key]
			for (key in raw_values) delete raw_values[key]
			for (key in lines) delete lines[key]
			for (key in names) delete names[key]
			count = 0; section = ""; id = ""; protocol = ""; managed = 0
		}
		function deprecated(name) {
			return name ~ /^crs_gfw(_fakedns|_proxy_tag)?$/ || name ~ /^crs_bilibili_hmt(_fakedns|_proxy_tag)?$/
		}
		function gfw_for_proxy(name) {
			if (name == "crs_proxy") return "crs_gfw"
			if (name == "crs_proxy_fakedns") return "crs_gfw_fakedns"
			if (name == "crs_proxy_proxy_tag") return "crs_gfw_proxy_tag"
			return ""
		}
		function append_proxy(proxy, gfw) {
			if (values[proxy] == "" && values[gfw] != "")
				printf "\toption %s %s\n", proxy, raw_values[gfw]
		}
		function emit( i, name, source) {
			if (count == 0) return
			if (section == "shunt_rules" && (managed || id ~ /^c2p_/)) { reset(); return }
			if (section != "nodes" || protocol != "_shunt") {
				for (i = 1; i <= count; i++) print lines[i]
				reset(); return
			}
			for (i = 1; i <= count; i++) {
				name = names[i]
				if (deprecated(name)) continue
				source = gfw_for_proxy(name)
				if (source != "" && values[name] == "" && values[source] != "") continue
				print lines[i]
			}
			append_proxy("crs_proxy", "crs_gfw")
			append_proxy("crs_proxy_fakedns", "crs_gfw_fakedns")
			append_proxy("crs_proxy_proxy_tag", "crs_gfw_proxy_tag")
			reset()
		}
		$1 == "config" {
			emit()
			count++; lines[count] = $0; names[count] = ""
			section = unquote($2); id = unquote($3)
			next
		}
		{
			count++; lines[count] = $0; names[count] = ""
			if ($1 == "option") {
				name = unquote($2); names[count] = name
				raw = $0
				sub(/^[ \t]*option[ \t]+[^ \t]+[ \t]+/, "", raw)
				raw_values[name] = raw; values[name] = unquote(raw)
				if (name == "protocol") protocol = values[name]
				if (name == "managed_by" && values[name] == "clash-rules-srs") managed = 1
			}
		}
		END { emit() }
	' "$STAGE"
}
```

phase 3 原内联 filter AWK 整段替换为：

```sh
preflight_shunt_nodes
migrate_and_filter_config > "$STAGE.filtered"
mv "$STAGE.filtered" "$STAGE"
cat "$FRAGMENT" >> "$STAGE"
uci -c "$ROOT" -t "$STAGE_SAVEDIR" -q show passwall2 >/dev/null
```

后续仍由现有 `cp "$STAGE" "$CONF_NEW"` → atomic `mv` → `uci commit` 切换；任一下载、SHA、commit 或 final URL 步骤失败时，现有 `restore` 从 `$BACKUP` 恢复整个 config，因此节点 options、managed rules、URLs 与 DAT 同属一个回滚域。不要在预检前调用 live `uci set/delete`。

- [ ] **步骤 4：运行测试确认通过**

```bash
gofmt -w internal/passwall/installer_test.go
go test ./internal/passwall -count=1
```

预期：PASS；测试输出仍含原 5 阶段进度与回滚进度，未增加新的运行时命令依赖。

- [ ] **步骤 5：提交**

```bash
git add internal/passwall/install.sh.tmpl internal/passwall/installer_test.go
git commit -m "feat(T6): 事务迁移 Shunt 节点配置"
```

### 任务 7：14 组默认契约与 BilibiliHMT 清理

**文件**：
- 修改：`sources.yaml`
- 修改：`config/passwall2-groups.yaml`
- 修改：`internal/config/repository_contract_test.go`
- 修改：`internal/manifest/manifest.go`
- 修改：`internal/manifest/manifest_test.go`
- 修改：`internal/rules/custom_test.go`
- 修改：`internal/geosite/merge_test.go`
- 修改：`internal/geoip/config_test.go`
- 修改：`internal/shadowrocket/render_test.go`
- 修改：`internal/passwall/installer_test.go`
- 修改：`README.md`
- 修改：`context.md`
- 删除：`custom/geosite/BilibiliHMT.yaml`
- 删除：`custom/geoip/BilibiliHMT.yaml`
- 删除：`internal/rules/testdata/custom/geosite/BilibiliHMT.yaml`
- 创建：`internal/rules/testdata/custom/geosite/proxy.yaml`

**接口**：
- 消费：任务 3 generated tag 注册、任务 4 最终 DAT 生成、任务 5 Shadowrocket alias、任务 6 节点迁移。
- 产出：仓库默认 14 组；完整公共 gfw/proxy/direct source 仍保留；BilibiliHMT 只在 manifest forbidden 与防回归测试中出现。

- [ ] **步骤 1：写失败测试**

把 `internal/config/repository_contract_test.go` 的默认组断言替换为 spec 精确序列，并同时渲染 PassWall2 section ID：

```go
func Test默认组ID顺序与标签精确匹配(t *testing.T) {
	groups, err := config.LoadGroups("../../config/passwall2-groups.yaml")
	if err != nil { t.Fatal(err) }
	want := []model.Group{
		{ID: "reject", Remarks: "广告拦截", GeoSite: []string{"reject"}, GeoIP: []string{}},
		{ID: "youtube", Remarks: "YouTube", GeoSite: []string{"youtube"}, GeoIP: []string{}},
		{ID: "netflix", Remarks: "Netflix", GeoSite: []string{"netflix"}, GeoIP: []string{"netflix"}},
		{ID: "spotify", Remarks: "Spotify", GeoSite: []string{"spotify"}, GeoIP: []string{}},
		{ID: "tiktok", Remarks: "TikTok", GeoSite: []string{"tiktok"}, GeoIP: []string{}},
		{ID: "ai", Remarks: "AI 服务", GeoSite: []string{"ai"}, GeoIP: []string{}},
		{ID: "apple_services", Remarks: "苹果服务", GeoSite: []string{"apple", "icloud"}, GeoIP: []string{}},
		{ID: "telegram", Remarks: "Telegram", GeoSite: []string{}, GeoIP: []string{"telegram"}},
		{ID: "google", Remarks: "Google 服务", GeoSite: []string{"google"}, GeoIP: []string{}},
		{ID: "proxy", Remarks: "代理规则", GeoSite: []string{targets.ProxyResidualTag}, GeoIP: []string{}},
		{ID: "tld_not_cn", Remarks: "非中国域名", GeoSite: []string{"tld-not-cn"}, GeoIP: []string{}},
		{ID: "private", Remarks: "私有网络", GeoSite: []string{"private"}, GeoIP: []string{"private"}},
		{ID: "cn", Remarks: "中国大陆", GeoSite: []string{"cn"}, GeoIP: []string{"cn"}},
		{ID: "direct", Remarks: "直连规则", GeoSite: []string{targets.DirectResidualTag}, GeoIP: []string{}},
	}
	if !reflect.DeepEqual(groups, want) { t.Fatalf("groups mismatch\ngot=%#v\nwant=%#v", groups, want) }
	fragment, err := passwall.Render(groups)
	if err != nil { t.Fatal(err) }
	if !bytes.Contains(fragment, []byte("config shunt_rules 'crs_proxy'")) ||
		!bytes.Contains(fragment, []byte("config shunt_rules 'crs_direct'")) ||
		bytes.Contains(fragment, []byte("crs_gfw")) || bytes.Contains(fragment, []byte("crs_bilibili_hmt")) {
		t.Fatalf("fragment=%s", fragment)
	}
}
```

把 custom 模板测试改名为 `Test默认Custom模板审计忽略生成型标签`，`defaultTemplateCases` 同时从 `sources.yaml` outputs 与 groups 收集目标，遇到 `targets.IsGenerated(side, tag)` 直接跳过。这样 gfw/proxy/direct 完整公共标签虽不再都由组直接引用，其 custom 模板仍被审计；两个残差无模板仍通过。测试末尾加入：

```go
for _, tag := range targets.GeneratedGeoSiteTags() {
	path := filepath.Join("../../custom/geosite", tag+".yaml")
	if _, err := os.Stat(path); !os.IsNotExist(err) { t.Fatalf("generated template %s must not exist: %v", path, err) }
}
for _, test := range defaultTemplateCases(t) {
	for _, tag := range targets.GeneratedGeoSiteTags() {
		if strings.Contains(test.path, tag+".yaml") { t.Fatalf("generated target entered template audit: %s", test.path) }
	}
}
```

用户手工加入 generated custom 后的拒绝路径由任务 3 的 `TestGenerated标签拒绝Custom输入` 提供失败测试。

新增仓库残留测试：

```go
func TestBilibiliHMT仓库契约已移除(t *testing.T) {
	paths := []string{
		"../../sources.yaml", "../../config/passwall2-groups.yaml", "../../README.md", "../../context.md",
		"../../custom/geosite", "../../custom/geoip", "../rules/testdata",
	}
	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil { return walkErr }
			if entry.IsDir() { return nil }
			data, err := os.ReadFile(path)
			if err != nil { return err }
			for _, forbidden := range []string{"BilibiliHMT", "bilibili_hmt", "xiaolin-bilibili"} {
				if bytes.Contains(data, []byte(forbidden)) { t.Errorf("%s still contains %s", path, forbidden) }
			}
			return nil
		})
		if err != nil { t.Fatal(err) }
	}
}
```

更新 `manifest_test.go`：默认 required GeoSite 必含两个 residual；Forbidden GeoSite/GeoIP 都含 `BilibiliHMT`，两侧仍含 `xiaolin-bilibili`。更新 Shadowrocket 默认组测试期望 14，并断言非中国域名 alias 行。更新 installer 默认 fragment 数量期望 14。把 geoip 排序 fixture 的 Bilibili tag 换成普通 `alpha`，把 geosite/rules 测试改成验证完整公共 `proxy` custom，不再用已删除契约作为一般功能样例。

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/config ./internal/manifest ./internal/rules ./internal/geosite ./internal/geoip ./internal/shadowrocket ./internal/passwall -count=1
```

预期：FAIL；默认配置仍有 16 组、Bilibili source/templates/docs，manifest 尚未禁止双侧 BilibiliHMT。

- [ ] **步骤 3：写最小实现**

`config/passwall2-groups.yaml` 完整替换为：

```yaml
# 数组顺序即托管分流优先级；每组必须显式包含 geosite 与 geoip。
groups:
  - id: reject
    remarks: 广告拦截
    geosite: [reject]
    geoip: []
  - id: youtube
    remarks: YouTube
    geosite: [youtube]
    geoip: []
  - id: netflix
    remarks: Netflix
    geosite: [netflix]
    geoip: [netflix]
  - id: spotify
    remarks: Spotify
    geosite: [spotify]
    geoip: []
  - id: tiktok
    remarks: TikTok
    geosite: [tiktok]
    geoip: []
  - id: ai
    remarks: AI 服务
    geosite: [ai]
    geoip: []
  - id: apple_services
    remarks: 苹果服务
    geosite: [apple, icloud]
    geoip: []
  - id: telegram
    remarks: Telegram
    geosite: []
    geoip: [telegram]
  - id: google
    remarks: Google 服务
    geosite: [google]
    geoip: []
  - id: proxy
    remarks: 代理规则
    geosite: [crs-proxy-residual]
    geoip: []
  - id: tld_not_cn
    remarks: 非中国域名
    geosite: [tld-not-cn]
    geoip: []
  - id: private
    remarks: 私有网络
    geosite: [private]
    geoip: [private]
  - id: cn
    remarks: 中国大陆
    geosite: [cn]
    geoip: [cn]
  - id: direct
    remarks: 直连规则
    geosite: [crs-direct-residual]
    geoip: []
```

从 `sources.yaml` 只删除完整 `xiaolin-bilibili` block；保留 `loyalsoldier-gfw`、`loyalsoldier-proxy`、`loyalsoldier-direct`，因为它们继续生成完整公共标签。

`manifest.go` 导入 `internal/targets`，将 required 初始化改为：

```go
requiredSite := map[string]struct{}{
	"cn": {},
	targets.ProxyResidualTag: {},
	targets.DirectResidualTag: {},
}
```

并向 `legacyForbidden.GeoSite`、`legacyForbidden.GeoIP` 各加入 `"BilibiliHMT"`；已有 `"xiaolin-bilibili"` 两侧项保留，防止旧 tag 回归。

`defaultTemplateCases` 的目标收集次序固定为：先 sources outputs、再 groups refs，`seen` 去重、最终按 path 排序：

```go
sources, err := config.LoadSources("../../sources.yaml")
if err != nil { t.Fatal(err) }
for _, source := range sources {
	if source.Outputs.GeoSite != nil { add(model.GeoSite, []string{source.Outputs.GeoSite.Tag}, siteMarkers, "DOMAIN-SUFFIX,example.com", 1, 0) }
	if source.Outputs.GeoIP != nil { add(model.GeoIP, []string{source.Outputs.GeoIP.Tag}, ipMarkers, "IP-CIDR,192.0.2.0/24", 0, 1) }
}
for _, group := range groups {
	add(model.GeoSite, group.GeoSite, siteMarkers, "DOMAIN-SUFFIX,example.com", 1, 0)
	add(model.GeoIP, group.GeoIP, ipMarkers, "IP-CIDR,192.0.2.0/24", 0, 1)
}
```

`add` 循环开头加入：

```go
if targets.IsGenerated(side, tag) { continue }
```

删除三份 Bilibili custom/fixture 文件，创建 `internal/rules/testdata/custom/geosite/proxy.yaml`：

```yaml
payload:
  - DOMAIN-SUFFIX,example.test
```

README/context 必须明确写出：

- 构建两次编译并保留完整 gfw/proxy/direct，同时仅托管组引用 residual。
- 默认 14 组精确顺序；gfw 已并入代理规则，原港澳台专用来源与双侧标签已破坏性删除且不保留兼容入口。
- proxy 与 tld-not-cn 必须同出口；Shadowrocket 由组引用绑定。
- installer 会预检所有 Shunt 节点，三族迁移/冲突失败/单侧启用警告/废弃字段清理均在同一事务。
- private/cn/direct 仍直连，reject 不变；`IPOnDemand` 不在本轮验收。

- [ ] **步骤 4：运行测试确认通过**

```bash
gofmt -w internal/config/repository_contract_test.go internal/manifest internal/rules/custom_test.go internal/geosite/merge_test.go internal/geoip/config_test.go internal/shadowrocket/render_test.go internal/passwall/installer_test.go
go test ./internal/config ./internal/manifest ./internal/rules ./internal/geosite ./internal/geoip ./internal/shadowrocket ./internal/passwall -count=1
! rg -n 'BilibiliHMT|bilibili_hmt|xiaolin-bilibili' sources.yaml config/passwall2-groups.yaml custom README.md context.md internal/rules/testdata
```

预期：Go tests PASS；最后一条 `rg` 退出 1 且无输出。manifest 与防回归测试内的禁止字符串不在该扫描范围，属于有意保留；integration fixture 在任务 8 同步清理并单独扫描。

- [ ] **步骤 5：提交**

```bash
git add sources.yaml config/passwall2-groups.yaml custom internal/config internal/manifest internal/rules internal/geosite/merge_test.go internal/geoip/config_test.go internal/shadowrocket/render_test.go internal/passwall/installer_test.go README.md context.md
git commit -m "feat(T7): 收敛十四组并移除 BilibiliHMT"
```

### 任务 8：真实 compiler 集成与发布门禁

**文件**：
- 修改：`internal/app/build_integration_test.go`
- 修改：`internal/app/testdata/sources.yaml`
- 修改：`internal/app/testdata/groups.yaml`
- 修改：`internal/app/testdata/community/google`
- 修改：`internal/app/testdata/community/cn`
- 创建：`internal/app/testdata/community/apple`
- 创建：`internal/app/testdata/community/icloud`
- 创建：`internal/app/testdata/community/private`
- 创建：`internal/app/testdata/community/spotify`
- 创建：`internal/app/testdata/community/tiktok`
- 删除：`internal/app/testdata/community/bilibili`
- 删除：`internal/app/testdata/custom/geosite/BilibiliHMT.yaml`
- 创建：`internal/app/testdata/custom/geosite/proxy.yaml`

**接口**：
- 消费：任务 1–7 的完整生产 pipeline。
- 产出：用固定本地 HTTPS source 响应与真实 pinned compiler 验证中间 DAT、最终 DAT、manifest、installer、Shadowrocket 与事务 Abort。

- [ ] **步骤 1：写失败测试**

把 `internal/app/testdata/groups.yaml` 完整替换为：

```yaml
groups:
  - {id: reject, remarks: 广告拦截, geosite: [reject], geoip: []}
  - {id: youtube, remarks: YouTube, geosite: [youtube], geoip: []}
  - {id: netflix, remarks: Netflix, geosite: [netflix], geoip: [netflix]}
  - {id: spotify, remarks: Spotify, geosite: [spotify], geoip: []}
  - {id: tiktok, remarks: TikTok, geosite: [tiktok], geoip: []}
  - {id: ai, remarks: AI 服务, geosite: [ai], geoip: []}
  - {id: apple_services, remarks: 苹果服务, geosite: [apple, icloud], geoip: []}
  - {id: telegram, remarks: Telegram, geosite: [], geoip: [telegram]}
  - {id: google, remarks: Google 服务, geosite: [google], geoip: []}
  - {id: proxy, remarks: 代理规则, geosite: [crs-proxy-residual], geoip: []}
  - {id: tld_not_cn, remarks: 非中国域名, geosite: [tld-not-cn], geoip: []}
  - {id: private, remarks: 私有网络, geosite: [private], geoip: [private]}
  - {id: cn, remarks: 中国大陆, geosite: [cn], geoip: [cn]}
  - {id: direct, remarks: 直连规则, geosite: [crs-direct-residual], geoip: []}
```

`internal/app/testdata/sources.yaml` 完整替换为六个 create source；其余前序标签由 community fixture 提供：

```yaml
sources:
  - id: fixture-reject
    behavior: domain
    url: "{{SERVER}}/reject"
    outputs: {geosite: {tag: reject, mode: create}}
  - id: fixture-gfw
    behavior: domain
    url: "{{SERVER}}/gfw"
    outputs: {geosite: {tag: gfw, mode: create}}
  - id: fixture-proxy
    behavior: domain
    url: "{{SERVER}}/proxy"
    outputs: {geosite: {tag: proxy, mode: create}}
  - id: fixture-direct
    behavior: domain
    url: "{{SERVER}}/direct"
    outputs: {geosite: {tag: direct, mode: create}}
  - id: fixture-tld-not-cn
    behavior: domain
    url: "{{SERVER}}/tld-not-cn"
    outputs: {geosite: {tag: tld-not-cn, mode: create}}
  - id: fixture-ai
    behavior: domain
    url: "{{SERVER}}/ai"
    outputs: {geosite: {tag: ai, mode: create}}
```

base GeoIP fixture补入 `telegram`。

在 `Test完整构建得到八文件且Checksum匹配` 中使用以下 source payload：

```go
sourcePayloads := map[string]string{
	"/reject": "payload:\n  - '+.ads.example'\n",
	"/gfw": "payload:\n  - '+.shared.proxy.example'\n  - '+.blocked.example'\n",
	"/proxy": "payload:\n  - '+.shared.proxy.example'\n  - 'www.youtube.example'\n  - '+.kept-proxy.example'\n",
	"/direct": "payload:\n  - 'covered.foreign'\n  - 'covered.cn'\n  - 'kept-direct.example'\n",
	"/tld-not-cn": "payload:\n  - '+.foreign'\n",
	"/ai": "payload:\n  - '+.ai-source.example'\n",
}
```

community `google` 固定为：

```text
domain:googleapis.com @cn
include:youtube
```

其余 fixture 文件固定为：

```text
# internal/app/testdata/community/youtube
domain:youtube.example

# internal/app/testdata/community/netflix
domain:netflix.example

# internal/app/testdata/community/spotify
domain:spotify.example

# internal/app/testdata/community/tiktok
domain:tiktok.example

# internal/app/testdata/community/apple
domain:apple.example

# internal/app/testdata/community/icloud
domain:icloud.example

# internal/app/testdata/community/private
domain:private

# internal/app/testdata/community/cn
domain:cn
```

`internal/app/testdata/custom/geosite/proxy.yaml` 固定为：

```yaml
payload:
  - DOMAIN-SUFFIX,custom-proxy.example
```

删除 `community/bilibili` 与旧 Bilibili custom fixture。

在完整构建返回后增加逐 Scenario 断言：

```go
intermediateBytes, err := os.ReadFile(filepath.Join(root, "build", "intermediate-geosite", "geosite.dat"))
if err != nil { t.Fatal(err) }
finalBytes, err := os.ReadFile(filepath.Join(root, "publish", "geosite.dat"))
if err != nil { t.Fatal(err) }
intermediate, err := geodat.DecodeGeoSite(intermediateBytes)
if err != nil { t.Fatal(err) }
final, err := geodat.DecodeGeoSite(finalBytes)
if err != nil { t.Fatal(err) }

t.Run("include 与属性在残差输入中已展开", func(t *testing.T) {
	google := intermediate["google"].Domains
	if !hasDomainRule(google, model.DomainSuffix, "youtube.example", nil) ||
		!hasDomainRule(google, model.DomainSuffix, "googleapis.com", []string{"@cn"}) {
		t.Fatalf("google=%#v", google)
	}
	for _, tag := range targets.GeneratedGeoSiteTags() {
		if len(final[tag].Domains) == 0 { t.Fatalf("generated %s=%#v", tag, final[tag]) }
	}
})

t.Run("完整标签未被残差替换", func(t *testing.T) {
	for _, tag := range []string{"gfw", "proxy", "direct"} {
		if !reflect.DeepEqual(intermediate[tag].Domains, final[tag].Domains) { t.Fatalf("%s changed", tag) }
	}
})

t.Run("reject 指纹不受优化影响", func(t *testing.T) {
	if !reflect.DeepEqual(intermediate["reject"].Domains, final["reject"].Domains) { t.Fatal("reject changed") }
	groups, err := config.LoadGroups(filepath.Join(root, "groups.yaml"))
	if err != nil || groups[0].ID != "reject" { t.Fatalf("groups=%#v err=%v", groups, err) }
})

prober := verify.NewProber(runner, filepath.Join(root, "publish", "geosite.dat"), filepath.Join(root, "publish", "geoip.dat"))
t.Run("中间 DAT 不污染最终探测缓存", func(t *testing.T) {
	for _, tag := range targets.GeneratedGeoSiteTags() {
		present, err := prober.Has(context.Background(), model.GeoSite, tag)
		if err != nil || !present { t.Fatalf("%s present=%v err=%v", tag, present, err) }
	}
})

t.Run("最终 DAT 不含 BilibiliHMT", func(t *testing.T) {
	for _, side := range []model.Side{model.GeoSite, model.GeoIP} {
		present, err := prober.Has(context.Background(), side, "BilibiliHMT")
		if err != nil || present { t.Fatalf("%s present=%v err=%v", side, present, err) }
	}
})
```

在 `TestProductionDependenciesFailuresPreservePreviousGeneration` 把旧 source HTTP case 改为 `fail: "/proxy"`、期望 `fixture-proxy`/`404`；把 strict custom case 改写 `custom/geosite/proxy.yaml`。再向 cases 加：

```go
{
	name: "generated source collision",
	mutate: func(t *testing.T, root string) {
		path := filepath.Join(root, "sources.yaml")
		data, err := os.ReadFile(path)
		if err != nil { t.Fatal(err) }
		data = []byte(strings.Replace(string(data), "{tag: proxy, mode: create}", "{tag: crs-proxy-residual, mode: create}", 1))
		if err := os.WriteFile(path, data, 0o600); err != nil { t.Fatal(err) }
	},
	want: []string{"fixture-proxy", "crs-proxy-residual", "generated"},
},
{
	name: "generated community collision",
	mutate: func(t *testing.T, root string) {
		path := filepath.Join(root, "community", "crs-direct-residual")
		if err := os.WriteFile(path, []byte("domain:collision.example\n"), 0o600); err != nil { t.Fatal(err) }
	},
	want: []string{"crs-direct-residual", "generated", "base/community"},
},
```

旧 `create collision` case 改为在 `community/proxy` 写 `domain:base-collision.example`，期望错误含 `fixture-proxy`、`geosite:proxy`、`create`；不能再替换已经不存在的 google source。完整构建与失败矩阵共用一个 `integrationSourcePayloads()` helper 返回上述六个 endpoint，避免两份 payload 漂移。

每个 case 继续调用既有 `seedIntegrationGeneration` 与 `assertIntegrationGeneration`，从而证明旧 build/publish marker 零变化。false/int/unknown attribute 无法由正常 domain-list 文本生成，保留任务 1 的直接 wire unit 失败测试；本 integration 只证明 compiler 的 bool-true 正常路径，不伪造 compiler 不会产生的输入。

Shadowrocket conf 断言 14 个策略组、`非中国域名 = select,代理规则,policy-select-name=代理规则`、不含 GFW/BilibiliHMT，并确认 installer fragment 仍有 `crs_proxy`/`crs_direct` section 且 domain_list 指向两个 residual tag。

- [ ] **步骤 2：运行测试确认失败**

```bash
go test -tags=integration ./internal/app -count=1
```

预期：FAIL；旧 fixture 仍要求 BilibiliHMT，且尚未证明两个 generated tag、完整标签指纹和 14 组发布物。

- [ ] **步骤 3：写最小实现**

按步骤 1 的精确 fixture/断言修改集成测试，并完成这些辅助函数：

```go
func hasDomainRule(rules []model.DomainRule, kind model.DomainKind, value string, attrs []string) bool {
	for _, rule := range rules {
		if rule.Kind == kind && rule.Value == value && reflect.DeepEqual(rule.Attrs, attrs) { return true }
	}
	return false
}
```

`buildSyntheticBaseGeoIP` 的 map 加 `"telegram": "149.154.160.0/20\n"`，输入名排序改为 `[]string{"cn", "netflix", "private", "telegram"}`。`newIntegrationRoot` 只复制 `testdata/custom/geosite/proxy.yaml`。原 strict custom schema failure 改写该 proxy 文件并期望 `proxy.yaml`，不再创建 Bilibili fixture。

manifest 断言改为：required GeoSite 含两个 residual 与完整 source tags；forbidden 双侧含 `BilibiliHMT`/`xiaolin-bilibili`；required/forbidden probe 都通过。保留八资产集合与 checksum、latest update URL、YouTube-before-Google、发布 tag 不进入 conf 的既有断言。

- [ ] **步骤 4：运行测试确认通过**

```bash
gofmt -w internal/app/build_integration_test.go
go test -tags=integration ./internal/app -count=1
go test ./internal/... -count=1
! rg -n 'BilibiliHMT|bilibili_hmt|xiaolin-bilibili' internal/app/testdata
git diff --check
```

预期：全部 PASS；真实 compiler 跑了两次 GeoSite、一次 GeoIP，正式 publish 是八资产闭集，旧 generation 在所有新失败路径下均保留。

- [ ] **步骤 5：提交**

```bash
git add internal/app/build_integration_test.go internal/app/testdata
git commit -m "test(T8): 验证残差发布流水线"
```

## 测试、验收与收尾

### 任务 9：验收（acceptance-qa）

> 本任务由 executing-plans 收尾审查阶段触发 acceptance-qa 按下表执行，不参与逐任务连续执行；报告与证据落盘特性目录 `acceptance/` 子目录。基线固定为实施前代码提交 `f2276af`，候选固定为进入验收时的功能分支 HEAD；两者必须消费同一份带 SHA-256 清单的 source/community/base-GeoIP/custom 快照。

| Scenario / 检查项 | 维度 | 执行方式 | 目标 | 阈值/预期 | 验收证据 |
|-------------------|------|---------|------|----------|---------|
| Xray AsIs 域名路由等价 | e2e-router | 验收任务 (D) | 用户授权的同一台 PassWall2 路由器、同一 Shunt 节点，`domainStrategy='AsIs'`；依次安装基线/候选 | 保留 14 组语料逐请求命中组与最终 outbound 全相同；未命中域名也相同；不得用 GeoIP 补救域名差异 | `acceptance/router-xray-asis-baseline.jsonl`、`router-xray-asis-candidate.jsonl`、逐请求 diff 与 UCI/core 版本快照 |
| Xray IPIfNonMatch 二次 IP 匹配等价 | e2e-router | 验收任务 (D) | 同一设备/节点切到 `domainStrategy='IPIfNonMatch'`，加入域名未命中后解析至 private/cn 的固定语料 | 域名首轮决策、解析 IP、二次匹配组、最终 outbound 四列逐请求一致 | `acceptance/router-xray-ipifnonmatch-*.jsonl` 与两轮匹配 diff |
| sing-box 原生路由等价 | e2e-router | 验收任务 (D) | 同一设备的 sing-box Shunt 节点及同一语料；记录其原生 route 配置，不把 Xray 字段名套入报告 | 每个请求的命中 rule-set/group 与最终 outbound 一致 | `acceptance/router-singbox-*.jsonl`、生成配置摘要与 diff |
| 静态规则规模下降至少 30% | perf-static | 验收任务 (D) | `f2276af` detached baseline worktree 与候选 worktree；同一不可变输入快照分别构建八资产 | 候选活动域名规则总数 ≤ 基线 70%；Shadowrocket `[Rule]` 行数 ≤ 70%；`[Rule]` UTF-8 字节数 ≤ 70% | `acceptance/static-metrics.json`，含两侧绝对值、比率、输入快照 hash、构建 commit；计数定义按 spec 术语表 |
| 三种运行矩阵 RSS 下降至少 5% | perf-router | 验收任务 (D) | Xray AsIs、Xray IPIfNonMatch、sing-box 三格；同设备/节点/语料，固定冷启动、稳定等待和流量预热 | 每格基线与候选各采 5 次同口径 RSS；候选中位数 ≤ 自身基线 95%；任一格未达标即 FAIL | `acceptance/rss-samples.csv`（30 个原始样本）、`rss-summary.json`（中位数/比率）、进程命令与采样时刻 |
| 仓库完整回归 | regression | 验收任务 (D) | 功能分支 worktree、本地真实 pinned 工具 | `go test ./...`、integration、`go vet ./...`、完整 build、四份 `sha256sum -c`、releasecmp、`git diff --check` 全部退出 0；publish 恰为八文件 | `acceptance/local-regression.log`、`publish-inventory.txt`、checksum 输出、工具版本 |
| Shadowrocket 真机导入 | e2e-device | 验收任务 (D) | iPhone Shadowrocket 导入候选 `shadowrocket.conf` | 本轮仍为 DEFERRED；不得记 PASS，不阻塞本特性其余验收 | `acceptance/runtime-rule-residuals-acceptance.md` 中单列 DEFERRED、原因与后续入口 |

- [ ] **步骤 1：建立同源基线并锁定输入**

```bash
git worktree add --detach .worktrees/runtime-rule-residuals-baseline f2276af
mkdir -p .worktrees/runtime-rule-residuals-source-snapshot
```

用仓库现有 `config.LoadSources` 与 `fetch.Client` 在一次受控采集里保存基线 sources 的全部响应（候选使用其中仍存在的同一 URL 子集）及 `config/geoip.base.json` 指向的 base DAT；community 固定为同一 bootstrap 目录快照。custom 使用同一个语义快照：复制当前 custom，但不包含已删除且本来为空的港澳台模板；旧基线允许该模板缺失。生成 `SHA256SUMS` 后只允许只读 replay，基线/候选构建日志必须逐 URL 记录同一个 snapshot SHA。若无法证明两侧共有输入字节相同，静态与路由验收不得开始。

- [ ] **步骤 2：执行本地回归与静态计数**

在候选 worktree：

```bash
go test ./...
go test -tags=integration ./internal/app
go vet ./...
go run ./cmd/geodata-build build --repo FlameMida/c2p-rules --release-tag acceptance-candidate
sha256sum -c publish/geosite.dat.sha256sum
sha256sum -c publish/geoip.dat.sha256sum
sha256sum -c publish/install_passwall2_rules.sh.sha256sum
sha256sum -c publish/shadowrocket.conf.sha256sum
go test ./internal/releasecmp -count=1
git diff --check
```

基线/候选活动规则计数都用任务 1 decoder 展开对应 groups 实际引用：组内按 `kind/value/normalized attrs` 去重后求和、跨组重复分别计数；Shadowrocket 只统计 `[Rule]` header 后、`GEOIP,CN`/`FINAL` 之前的行及其原始 UTF-8 字节。报告用整数交叉乘法判断 `candidate*100 <= baseline*70`，避免浮点舍入掩盖失败。

- [ ] **步骤 3：执行三种路由矩阵与 RSS 采样**

开始前只读记录设备型号、固件、PassWall2/Xray/sing-box 版本、当前 `/etc/config/passwall2` SHA-256、活动 Shunt node ID 与三族字段；在路由器执行下列精确备份（`ASSET_DIR` 来自设备当前 UCI，不从主机猜）：

```sh
ACCEPTANCE_BACKUP=/tmp/passwall2-runtime-rule-residuals-acceptance
ASSET_DIR=$(uci -q get 'passwall2.@global_rules[0].v2ray_location_asset')
mkdir -p "$ACCEPTANCE_BACKUP"
cp /etc/config/passwall2 "$ACCEPTANCE_BACKUP/passwall2"
cp "$ASSET_DIR/geosite.dat" "$ACCEPTANCE_BACKUP/geosite.dat"
cp "$ASSET_DIR/geoip.dat" "$ACCEPTANCE_BACKUP/geoip.dat"
printf '%s\n' "$ASSET_DIR" > "$ACCEPTANCE_BACKUP/asset-dir"
sha256sum "$ACCEPTANCE_BACKUP/passwall2" "$ACCEPTANCE_BACKUP/geosite.dat" "$ACCEPTANCE_BACKUP/geoip.dat" > "$ACCEPTANCE_BACKUP/SHA256SUMS"
```

每格固定流程为：停止目标 core → 安装该侧 DAT/managed rules → 启动 → 等待相同稳定时长 → 发同一 corpus 预热 → 记录命中/出口 → 以相同命令采 RSS 5 次。基线与候选交替顺序固定为 `baseline → candidate`，三格之间恢复同一初始 UCI 与 DAT；任何迁移 warning、冲突拒绝或恢复失败进入报告且停止后续设备写入。

- [ ] **步骤 4：写验收报告并提交证据**

`acceptance/runtime-rule-residuals-acceptance.md` 必须逐表格行写 `PASS`/`FAIL`/`BLOCKED`/`DEFERRED`，列出绝对值、比率、commit、输入 snapshot hash 与原始证据相对路径；不允许把本地 fake UCI 当设备 PASS。全部强制行 PASS 后：

```bash
git add .spec-dev/2026-08-16-runtime-rule-residuals/acceptance
git commit -m "test(T9): 记录残差优化验收"
```

若路由器不可达或没有设备写入授权，三种路由/RSS 行记 `BLOCKED` 并停在验收任务，不宣称本特性交付完成；Shadowrocket 真机行始终按 spec 记 `DEFERRED`。

### 任务 10：合并与清理

**资源台账**（清理依据；执行中创建即追加；行格式为“复选框、资源类型、精确标识、清理命令”）：

- [ ] worktree: `.worktrees/runtime-rule-residuals` / branch `plan/2026-08-16-runtime-rule-residuals` —— `git worktree remove .worktrees/runtime-rule-residuals && git branch -d plan/2026-08-16-runtime-rule-residuals`
- [ ] detached baseline worktree: `.worktrees/runtime-rule-residuals-baseline` —— `git worktree remove .worktrees/runtime-rule-residuals-baseline`
- [ ] temporary source snapshot: `.worktrees/runtime-rule-residuals-source-snapshot` —— 校验验收报告已记录 `SHA256SUMS` 后执行 `rm -r .worktrees/runtime-rule-residuals-source-snapshot`
- [ ] router config/DAT backup: `/tmp/passwall2-runtime-rule-residuals-acceptance` —— 读取其中 `asset-dir`，恢复 `/etc/config/passwall2`、`geosite.dat`、`geoip.dat`，执行 `uci commit passwall2` 并重启 PassWall2；回读三份 SHA-256 等于 `SHA256SUMS` 后执行 `rm -r /tmp/passwall2-runtime-rule-residuals-acceptance`

台账总则：**清理只遍历本台账、台账外一律不动**（可疑残留只报告不删）；共享缓存（`~/.cargo`、pnpm store、npm cache 等）默认保留，仅用户显式要求清理时才登记入账；台账限定持久资源（容器、测试库/表、临时目录、后台服务），worktree 内构建产物随 worktree 删除自然回收、不入账。

- [ ] **步骤 1：全量验证（安全网）与归属裁决**

在 worktree 内运行：

```bash
go test ./...
go test -tags=integration ./internal/app
go vet ./...
git diff --check
```

- 全绿 → 进入步骤 2。
- 失败测试在相关测试范围内 → 修复并复跑全绿后进入步骤 2。
- 失败测试在范围之外 → 归属裁决：在主工作区的源分支检出上复跑该测试（主工作区有未提交改动 → 先询问用户）。源分支同样失败 → 报告“既有失败”，请用户裁决是否阻塞合并，不自行静默忽略；源分支通过 → 判定为本次引入的回归，修复并复跑全绿后进入步骤 2。

- [ ] **步骤 2：测试退役检查**

扫描本计划“相关测试范围”内的测试，按 active spec 的现行 Scenario 名查找孤儿测试。重点候选是旧 `Test组名与顺序对齐YAML` 及所有以 BilibiliHMT 为一般能力的旧测试；任务 7/8 应已用新 Scenario 测试替换或删除。只有“测试名对不上任何 active spec 的现行 Scenario”且所属 Requirement 已 REMOVED/被 Superseded 标注两个条件同时成立才列为候选。候选非空 → 列清单征询用户，同意后删除并计入本任务提交；用户未确认则不删除。无候选 → 声明“无孤儿测试”。

- [ ] **步骤 3：取代回写**

本 spec 对 `.spec-dev/2026-08-15-shadowrocket-conf/spec/shadowrocket-conf-design.md` 是部分取代，旧 spec 保持 `status: active`：

1. 删除 H1 下 `Superseded-pending (2026-08-16)` 行。
2. 在旧 spec `### Requirement: 十六个具名策略组与默认出口` 标题下一行插入：

```markdown
> **Superseded (2026-08-16)** — 本 Requirement 的组数、独立 GFW/BilibiliHMT 组及 `tld_not_cn` 可独立选择出口的契约，已由 `.spec-dev/2026-08-16-runtime-rule-residuals/spec/runtime-rule-residuals-design.md` 的十四组与绑定出口 Requirement 取代；reject/cn/private/direct 的既有默认、remarks 唯一性、空组失败和不写节点分面继续有效。
```

3. 在旧成功标准后加注：16 组已被 14 组取代，八资产与 checksum 成功标准继续有效。
4. 在旧非目标“修改 PassWall2 安装器、UCI 托管组或两个 dat 的语义”后加注：该非目标仅适用于旧交付，本特性已明确取代。
5. 在旧关键决策“16 个具名 select 组”后加注：组数与 tld policy 由新 spec 取代，其余默认出口继续共存。
6. 在旧风险“组间不去重”后加注：完整公共标签仍不做跨组改写；默认运行组现由编译后残差进行有证明裁剪。

旧 spec 不翻 `superseded`、不填写 `superseded_by`；这是部分取代，不执行 covers 差集接管。

- [ ] **步骤 4：合并回来源分支**

先确认用户已明确授权合并/提交；没有授权则停下交接，不把计划阶段的 `ok` 推断为合并授权。获得授权后：

```bash
cd "$(dirname "$(git rev-parse --git-common-dir)")"
git merge plan/2026-08-16-runtime-rule-residuals
```

合并冲突、或主工作区有未提交改动 → 停下向计划作者确认，不强行合并。

- [ ] **步骤 5：清理（按资源台账逐条执行）**

先恢复并回读路由器配置，再逐条执行资源台账中的 source snapshot、baseline worktree、功能 worktree 清理命令并勾选。命令执行失败 → 该行保留未勾选并报告用户；资源已不存在 → 勾选并注明“已不存在”。台账外的文件、进程、设备数据一律不动。

- [ ] **步骤 6：sync_commit 锚定**

```bash
SYNC=$(git rev-parse HEAD)
# 将新 spec 的 sync_commit: null 更新为 $SYNC；旧 Shadowrocket spec 仍 active，也同步锚定为同一 $SYNC。
git add .spec-dev/2026-08-16-runtime-rule-residuals/spec/runtime-rule-residuals-design.md \
        .spec-dev/2026-08-15-shadowrocket-conf/spec/shadowrocket-conf-design.md
git commit -m "chore(spec): sync_commit 锚定 ${SYNC:0:7}"
```

此后按新 spec frontmatter 的 `sync_commit` 与 `covers` 逐项运行 `git diff`，所得即“spec 上次确认同步以来的代码变化”。非 git 仓库跳过。
