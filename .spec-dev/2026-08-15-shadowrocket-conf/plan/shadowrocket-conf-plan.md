# Shadowrocket 胖配置实施计划

> **执行方式**：使用 spec-dev 的 executing-plans skill 逐任务执行本计划；无该 skill 的环境直接从任务 0 起按序执行至最终任务。步骤用复选框（- [ ]）语法跟踪；脱离项目携带时连同特性目录（含 spec）整体带走。
>
> **偏差处理**：执行中发现计划与现实不符——小偏差（路径笔误、明显遗漏但意图清楚）就地修正并在提交信息中注明；接口、数据结构等契约级偏差停下向计划作者确认，不猜着改。

**目标**：让完整构建从最终 geosite.dat / geoip.dat 展开 16 个托管组，发布可供 Shadowrocket 远程导入和自动更新的胖 shadowrocket.conf 及其 checksum。

**Spec**：.spec-dev/2026-08-15-shadowrocket-conf/spec/shadowrocket-conf-design.md

**架构**：新增 internal/shadowrocket，以 Go 标准库直接读取最终 dat 的 protobuf wire 数据，按 groups 顺序形成 GroupRules，再渲染 [General]、[Proxy Group] 与 [Rule]。internal/app 在现有安装器之后写 conf 和 checksum，资产闭集、发布指纹、GitHub Workflow 与文档统一从六资产升级为八资产。

**技术栈**：Go 1.26.x 标准库（encoding/binary、net/netip）、现有事务工作区与 SHA-256 校验、GitHub Actions YAML、Go 单元测试及 integration build tag。

## 全局约束

- 生产构建保持单一 Go CLI；不新增 Python、Node、远程 list、GitHub Pages、module 或节点定义。
- 规则源只能是本次 staging publish/ 下编译完成的 geosite.dat 与 geoip.dat；不得读取 build/data-merged 或 build/ip 作为 conf 发布内容。
- publish/ 成功提交时必须恰有 8 个普通文件；Shadowrocket checksum 必须是 64 位小写十六进制、两个空格、basename、换行。
- group 数组顺序同时决定 [Proxy Group] 与 [Rule] 顺序；默认仓库配置必须生成全部 16 个 remarks。
- 默认出口按 group id：reject → REJECT；cn、private、direct → DIRECT；其他 → PROXY，并显式写 policy-select-name。
- regexp 跳过且不失败；属性不输出；组内只按 kind+value 去重，组间不去重。
- conf 不写 [Proxy]，不嵌入 RELEASE_TAG；update-url 固定使用 GitHub 官方 latest shadowrocket.conf URL，禁止镜像前缀。
- 旧 latest 只有六资产时 baseline 比较必须失败；首次上线由默认分支手动 force_publish=true，不做兼容降级。
- 真机导入、自动更新与大 conf 编译耗时为 DEFERRED，不得用本地测试冒充通过。

## 相关测试范围

仓库没有测试影响分析工具；按 spec covers 与直接受影响的契约测试，任务 0 基线运行：

~~~bash
go mod download
go test ./internal/app ./internal/verify ./internal/releasecmp
go test -tags=integration ./internal/app
~~~

internal/shadowrocket 在基线尚不存在，因此从任务 1 开始加入聚焦命令；最终任务仍运行 go test ./... 与 integration 全量安全网。

## 文件结构

- 创建 internal/shadowrocket/expand.go：解析最终 dat、映射 domain kind / CIDR、按组去重并暴露 Expand。
- 创建 internal/shadowrocket/expand_test.go：用最小 protobuf wire 夹具覆盖 include 后结果、GeoIP 底包、regexp、缺 tag 与空组。
- 创建 internal/shadowrocket/render.go：校验 repo / conf token，生成策略组、规则、latest update-url 与兜底。
- 创建 internal/shadowrocket/render_test.go：覆盖类型映射、16 组顺序、默认出口、空组、update-url 与末尾兜底。
- 创建 internal/shadowrocket/testdata/rules.golden：仅锁定 [Rule] 代表性输出，不把 [General] 可选键误当闭集。
- 修改 internal/app/build.go、internal/app/deps.go：增加 RenderShadowrocket finish stage，并把资产验证名称改成 VerifyReleaseAssets。
- 修改 internal/app/build_test.go、internal/app/build_integration_test.go、internal/app/testdata/groups.yaml：覆盖事务回滚、skip-compile、真实 dat 展开与八资产。
- 修改 internal/verify/assets.go、internal/verify/assets_test.go：资产权威清单从 6 扩到 8。
- 修改 internal/releasecmp/releasecmp.go、internal/releasecmp/releasecmp_test.go、internal/app/commands_test.go：指纹加入 conf 原始字节并拒绝旧六资产 baseline。
- 修改 internal/app/workflow_contract_test.go、.github/workflows/build.yml：构建、Artifact、草稿上传、下载回读与 checksum 全部升级为八资产。
- 修改 internal/app/docs_contract_test.go、README.md、context.md：写明八资产、三种导入方式、官方 update-url 与首次强制发布。
- 创建 .spec-dev/2026-08-15-shadowrocket-conf/acceptance/shadowrocket-device.md：仅在具备已发布 Release 与真机授权后记录 E2E；当前计划预期状态为 DEFERRED。

---

### 任务 0：建立隔离工作区

- [x] **步骤 1：检测已有隔离**

运行：git rev-parse --git-dir 与 git rev-parse --git-common-dir
两者不同、且 git rev-parse --show-superproject-working-tree 无输出（排除 submodule）
→ 已在隔离工作区，跳过本任务。

- [x] **步骤 2：建立 worktree**

有原生 worktree 工具（如 EnterWorktree）或 using-git-worktrees skill 时优先使用（Codex 无原生 worktree 工具，直接走下面的手工路径）；否则手工降级：
确认 .worktrees/ 已被忽略（git check-ignore -q .worktrees；当前 .gitignore 已包含该目录），然后运行：

~~~bash
git worktree add .worktrees/plan-2026-08-15-shadowrocket-conf -b plan/2026-08-15-shadowrocket-conf
cd .worktrees/plan-2026-08-15-shadowrocket-conf
~~~

- [x] **步骤 3：安装依赖并验证基线**

~~~bash
go mod download
go test ./internal/app ./internal/verify ./internal/releasecmp
go test -tags=integration ./internal/app
~~~

预期：三个命令退出码均为 0。任一基线测试失败则停下报告，先问再继续。

## 核心规则生成

### 任务 1：从最终 dat 展开托管组

**文件**：
- 创建：internal/shadowrocket/expand.go
- 创建：internal/shadowrocket/expand_test.go

**接口**：
- 消费：model.Group、model.DomainKind、context.Context、最终 geosite.dat / geoip.dat 文件路径。
- 产出：DomainLine、GroupRules，以及 Expand(ctx context.Context, siteDat, ipDat string, groups []model.Group) ([]GroupRules, error)。

- [x] **步骤 1：写失败测试**

创建 internal/shadowrocket/expand_test.go。测试夹具直接编码最终 dat 的 wire message，确保测试不借道 data-merged、build/ip 或 geoview：

~~~go
package shadowrocket_test

import (
	"context"
	"encoding/binary"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/shadowrocket"
)

func TestInclude被展开进Google组(t *testing.T) {
	site, ip := writeDatPair(t,
		geoSiteDat(siteEntry("GOOGLE",
			domainEntry(2, "google.example"),
			domainEntry(2, "youtube-from-include.example"),
		)),
		geoIPDat(),
	)
	groups := []model.Group{{ID: "google", Remarks: "Google 服务", GeoSite: []string{"google"}}}
	got, err := shadowrocket.Expand(context.Background(), site, ip, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !hasDomain(got[0].Domains, model.DomainSuffix, "youtube-from-include.example") {
		t.Fatalf("expanded=%+v", got)
	}
}

func TestMergeBaseGeoIP含底包(t *testing.T) {
	site, ip := writeDatPair(t,
		geoSiteDat(siteEntry("CN", domainEntry(2, "cn.example"))),
		geoIPDat(ipEntry("CN",
			cidrEntry(netip.MustParsePrefix("192.0.2.0/24")),
			cidrEntry(netip.MustParsePrefix("2001:db8::/32")),
		)),
	)
	groups := []model.Group{{ID: "cn", Remarks: "中国大陆", GeoSite: []string{"cn"}, GeoIP: []string{"cn"}}}
	got, err := shadowrocket.Expand(context.Background(), site, ip, groups)
	if err != nil {
		t.Fatal(err)
	}
	if want := []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("2001:db8::/32"),
	}; len(got[0].CIDRs) != len(want) || got[0].CIDRs[0] != want[0] || got[0].CIDRs[1] != want[1] {
		t.Fatalf("cidrs=%v want=%v", got[0].CIDRs, want)
	}
}

func TestRegexp被跳过(t *testing.T) {
	withAttribute := append(domainEntry(2, "kept.example"), fieldBytes(3, fieldBytes(1, []byte("cn")))...)
	site, ip := writeDatPair(t,
		geoSiteDat(siteEntry("TEST",
			withAttribute,
			domainEntry(1, "^x{1,3}\\.skipped\\.example$"),
			domainEntry(2, "kept.example"),
		)),
		geoIPDat(),
	)
	groups := []model.Group{{ID: "test", Remarks: "测试组", GeoSite: []string{"test"}}}
	got, err := shadowrocket.Expand(context.Background(), site, ip, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Domains) != 1 || got[0].Domains[0].Value != "kept.example" {
		t.Fatalf("domains=%+v", got[0].Domains)
	}
}

func TestExpand拒绝缺失Tag与空组(t *testing.T) {
	tests := map[string]struct {
		site []byte
		want string
	}{
		"missing": {geoSiteDat(), "geosite:not-exist"},
		"regexp only": {geoSiteDat(siteEntry("TEST", domainEntry(1, "only-regexp"))), "测试组"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			site, ip := writeDatPair(t, test.site, geoIPDat())
			tag := "test"
			if name == "missing" {
				tag = "not-exist"
			}
			_, err := shadowrocket.Expand(context.Background(), site, ip, []model.Group{{
				ID: "test", Remarks: "测试组", GeoSite: []string{tag},
			}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func writeDatPair(t *testing.T, siteData, ipData []byte) (string, string) {
	t.Helper()
	root := t.TempDir()
	site := filepath.Join(root, "geosite.dat")
	ip := filepath.Join(root, "geoip.dat")
	if err := os.WriteFile(site, siteData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ip, ipData, 0o600); err != nil {
		t.Fatal(err)
	}
	return site, ip
}

func geoSiteDat(entries ...[]byte) []byte {
	var result []byte
	for _, entry := range entries {
		result = append(result, fieldBytes(1, entry)...)
	}
	return result
}

func siteEntry(tag string, domains ...[]byte) []byte {
	result := fieldBytes(1, []byte(tag))
	for _, domain := range domains {
		result = append(result, fieldBytes(2, domain)...)
	}
	return result
}

func domainEntry(kind uint64, value string) []byte {
	result := fieldVarint(1, kind)
	return append(result, fieldBytes(2, []byte(value))...)
}

func geoIPDat(entries ...[]byte) []byte {
	var result []byte
	for _, entry := range entries {
		result = append(result, fieldBytes(1, entry)...)
	}
	return result
}

func ipEntry(tag string, cidrs ...[]byte) []byte {
	result := fieldBytes(1, []byte(tag))
	for _, cidr := range cidrs {
		result = append(result, fieldBytes(2, cidr)...)
	}
	return result
}

func cidrEntry(prefix netip.Prefix) []byte {
	address := prefix.Addr().AsSlice()
	result := fieldBytes(1, address)
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

func hasDomain(lines []shadowrocket.DomainLine, kind model.DomainKind, value string) bool {
	for _, line := range lines {
		if line.Kind == kind && line.Value == value {
			return true
		}
	}
	return false
}
~~~

- [x] **步骤 2：运行测试确认失败**

运行：

~~~bash
go test ./internal/shadowrocket -run 'Test(Include被展开进Google组|MergeBaseGeoIP含底包|Regexp被跳过|Expand拒绝缺失Tag与空组)$' -v
~~~

预期：FAIL，报 internal/shadowrocket 不存在或 Expand / DomainLine 未定义。

- [x] **步骤 3：写最小实现**

创建 internal/shadowrocket/expand.go。wireCursor 每次完整消费一个字段；未知普通 protobuf 字段会被消费，已知字段 wire type 错误、截断、非法 enum / IP / prefix、重复 tag 和缺 tag 都返回错误：

~~~go
package shadowrocket

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"unicode"

	"clash-rules-srs/internal/model"
)

type DomainLine struct {
	Kind  model.DomainKind
	Value string
}

type GroupRules struct {
	Group   model.Group
	Domains []DomainLine
	CIDRs   []netip.Prefix
}

type siteRecord struct {
	tag     string
	domains []DomainLine
}

type ipRecord struct {
	tag   string
	cidrs []netip.Prefix
}

func Expand(ctx context.Context, siteDat, ipDat string, groups []model.Group) ([]GroupRules, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	siteBytes, err := os.ReadFile(siteDat)
	if err != nil {
		return nil, fmt.Errorf("read geosite dat %s: %w", siteDat, err)
	}
	ipBytes, err := os.ReadFile(ipDat)
	if err != nil {
		return nil, fmt.Errorf("read geoip dat %s: %w", ipDat, err)
	}
	sites, err := decodeGeoSiteList(siteBytes)
	if err != nil {
		return nil, fmt.Errorf("decode geosite dat %s: %w", siteDat, err)
	}
	ips, err := decodeGeoIPList(ipBytes)
	if err != nil {
		return nil, fmt.Errorf("decode geoip dat %s: %w", ipDat, err)
	}

	result := make([]GroupRules, 0, len(groups))
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		expanded := GroupRules{Group: group}
		domainSeen := make(map[string]struct{})
		for _, tag := range group.GeoSite {
			record, ok := sites[strings.ToLower(tag)]
			if !ok {
				return nil, fmt.Errorf("group %q references missing geosite:%s", group.Remarks, tag)
			}
			for _, line := range record.domains {
				key := string(line.Kind) + "\x00" + line.Value
				if _, duplicate := domainSeen[key]; duplicate {
					continue
				}
				domainSeen[key] = struct{}{}
				expanded.Domains = append(expanded.Domains, line)
			}
		}
		cidrSeen := make(map[netip.Prefix]struct{})
		for _, tag := range group.GeoIP {
			record, ok := ips[strings.ToLower(tag)]
			if !ok {
				return nil, fmt.Errorf("group %q references missing geoip:%s", group.Remarks, tag)
			}
			for _, prefix := range record.cidrs {
				if _, duplicate := cidrSeen[prefix]; duplicate {
					continue
				}
				cidrSeen[prefix] = struct{}{}
				expanded.CIDRs = append(expanded.CIDRs, prefix)
			}
		}
		if len(expanded.Domains) == 0 && len(expanded.CIDRs) == 0 {
			return nil, fmt.Errorf("group %q (%s) expands to zero rules", group.Remarks, group.ID)
		}
		result = append(result, expanded)
	}
	return result, nil
}

func decodeGeoSiteList(data []byte) (map[string]siteRecord, error) {
	result := make(map[string]siteRecord)
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return nil, err
		}
		if field != 1 {
			continue
		}
		if wire != 2 {
			return nil, fmt.Errorf("geosite entry has wire type %d", wire)
		}
		record, err := decodeGeoSite(payload)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(record.tag)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate geosite tag %q", record.tag)
		}
		result[key] = record
	}
	return result, nil
}

func decodeGeoSite(data []byte) (siteRecord, error) {
	var record siteRecord
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return siteRecord{}, err
		}
		switch field {
		case 1:
			if wire != 2 {
				return siteRecord{}, fmt.Errorf("geosite tag has wire type %d", wire)
			}
			record.tag = string(payload)
		case 2:
			if wire != 2 {
				return siteRecord{}, fmt.Errorf("geosite domain has wire type %d", wire)
			}
			line, regexp, err := decodeDomain(payload)
			if err != nil {
				return siteRecord{}, fmt.Errorf("tag %q: %w", record.tag, err)
			}
			if !regexp {
				record.domains = append(record.domains, line)
			}
		}
	}
	if record.tag == "" {
		return siteRecord{}, fmt.Errorf("geosite entry has empty tag")
	}
	return record, nil
}

func decodeDomain(data []byte) (DomainLine, bool, error) {
	var kind uint64
	var value string
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil {
			return DomainLine{}, false, err
		}
		switch field {
		case 1:
			if wire != 0 {
				return DomainLine{}, false, fmt.Errorf("domain kind has wire type %d", wire)
			}
			kind = scalar
		case 2:
			if wire != 2 {
				return DomainLine{}, false, fmt.Errorf("domain value has wire type %d", wire)
			}
			value = string(payload)
		}
	}
	if kind == 1 {
		return DomainLine{}, true, nil
	}
	if !validRuleValue(value) {
		return DomainLine{}, false, fmt.Errorf("invalid domain value %q", value)
	}
	switch kind {
	case 0:
		return DomainLine{Kind: model.DomainKeyword, Value: value}, false, nil
	case 2:
		return DomainLine{Kind: model.DomainSuffix, Value: value}, false, nil
	case 3:
		return DomainLine{Kind: model.DomainFull, Value: value}, false, nil
	default:
		return DomainLine{}, false, fmt.Errorf("unsupported domain kind %d", kind)
	}
}

func decodeGeoIPList(data []byte) (map[string]ipRecord, error) {
	result := make(map[string]ipRecord)
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return nil, err
		}
		if field != 1 {
			continue
		}
		if wire != 2 {
			return nil, fmt.Errorf("geoip entry has wire type %d", wire)
		}
		record, err := decodeGeoIP(payload)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(record.tag)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate geoip tag %q", record.tag)
		}
		result[key] = record
	}
	return result, nil
}

func decodeGeoIP(data []byte) (ipRecord, error) {
	var record ipRecord
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return ipRecord{}, err
		}
		switch field {
		case 1:
			if wire != 2 {
				return ipRecord{}, fmt.Errorf("geoip tag has wire type %d", wire)
			}
			record.tag = string(payload)
		case 2:
			if wire != 2 {
				return ipRecord{}, fmt.Errorf("geoip cidr has wire type %d", wire)
			}
			prefix, err := decodeCIDR(payload)
			if err != nil {
				return ipRecord{}, fmt.Errorf("tag %q: %w", record.tag, err)
			}
			record.cidrs = append(record.cidrs, prefix)
		}
	}
	if record.tag == "" {
		return ipRecord{}, fmt.Errorf("geoip entry has empty tag")
	}
	return record, nil
}

func decodeCIDR(data []byte) (netip.Prefix, error) {
	var addressBytes []byte
	var bits uint64
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil {
			return netip.Prefix{}, err
		}
		switch field {
		case 1:
			if wire != 2 {
				return netip.Prefix{}, fmt.Errorf("cidr address has wire type %d", wire)
			}
			addressBytes = append([]byte(nil), payload...)
		case 2:
			if wire != 0 {
				return netip.Prefix{}, fmt.Errorf("cidr prefix has wire type %d", wire)
			}
			bits = scalar
		}
	}
	address, ok := netip.AddrFromSlice(addressBytes)
	if !ok || address.Is4In6() {
		return netip.Prefix{}, fmt.Errorf("invalid cidr address %x", addressBytes)
	}
	if bits > uint64(address.BitLen()) {
		return netip.Prefix{}, fmt.Errorf("invalid /%d for %s", bits, address)
	}
	return netip.PrefixFrom(address, int(bits)).Masked(), nil
}

func validRuleValue(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, ',') {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

type wireCursor struct {
	data []byte
}

func (c *wireCursor) more() bool {
	return len(c.data) != 0
}

func (c *wireCursor) next() (field, wire uint64, payload []byte, scalar uint64, err error) {
	key, n := binary.Uvarint(c.data)
	if n <= 0 {
		return 0, 0, nil, 0, fmt.Errorf("invalid protobuf field key")
	}
	c.data = c.data[n:]
	field, wire = key>>3, key&7
	if field == 0 {
		return 0, 0, nil, 0, fmt.Errorf("protobuf field number is zero")
	}
	switch wire {
	case 0:
		scalar, n = binary.Uvarint(c.data)
		if n <= 0 {
			return 0, 0, nil, 0, fmt.Errorf("invalid protobuf varint")
		}
		c.data = c.data[n:]
	case 1:
		if len(c.data) < 8 {
			return 0, 0, nil, 0, fmt.Errorf("truncated protobuf fixed64")
		}
		payload, c.data = c.data[:8], c.data[8:]
	case 2:
		length, lengthBytes := binary.Uvarint(c.data)
		if lengthBytes <= 0 {
			return 0, 0, nil, 0, fmt.Errorf("invalid protobuf byte length")
		}
		c.data = c.data[lengthBytes:]
		if length > uint64(len(c.data)) {
			return 0, 0, nil, 0, fmt.Errorf("truncated protobuf bytes")
		}
		payload, c.data = c.data[:int(length)], c.data[int(length):]
	case 5:
		if len(c.data) < 4 {
			return 0, 0, nil, 0, fmt.Errorf("truncated protobuf fixed32")
		}
		payload, c.data = c.data[:4], c.data[4:]
	default:
		return 0, 0, nil, 0, fmt.Errorf("unsupported protobuf wire type %d", wire)
	}
	return field, wire, payload, scalar, nil
}
~~~

- [x] **步骤 4：运行测试确认通过**

~~~bash
gofmt -w internal/shadowrocket/expand.go internal/shadowrocket/expand_test.go
go test ./internal/shadowrocket -run 'Test(Include被展开进Google组|MergeBaseGeoIP含底包|Regexp被跳过|Expand拒绝缺失Tag与空组)$' -v
~~~

预期：4 个测试 PASS；go.mod / go.sum 无变化。

- [x] **步骤 5：提交**

~~~bash
git add internal/shadowrocket/expand.go internal/shadowrocket/expand_test.go
git commit -m "feat(T1): 从最终 dat 展开 Shadowrocket 规则"
~~~

### 任务 2：渲染 Shadowrocket 配置

**文件**：
- 创建：internal/shadowrocket/render.go
- 创建：internal/shadowrocket/render_test.go
- 创建：internal/shadowrocket/testdata/rules.golden

**接口**：
- 消费：任务 1 的 []GroupRules 与构建参数 repo。
- 产出：Render(groups []GroupRules, repo string) ([]byte, error)，返回不含 RELEASE_TAG、可直接原子写盘的 UTF-8 conf。

- [x] **步骤 1：写失败测试**

创建 internal/shadowrocket/testdata/rules.golden：

~~~text
DOMAIN-SUFFIX,suffix.example,中国大陆
DOMAIN,full.example,中国大陆
DOMAIN-KEYWORD,needle,中国大陆
IP-CIDR,192.0.2.0/24,中国大陆,no-resolve
IP-CIDR6,2001:db8::/32,中国大陆,no-resolve
GEOIP,CN,中国大陆
FINAL,PROXY
~~~

创建 internal/shadowrocket/render_test.go：

~~~go
package shadowrocket_test

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/config"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/shadowrocket"
)

func Test四种可映射类型与CIDR(t *testing.T) {
	groups := []shadowrocket.GroupRules{{
		Group: model.Group{ID: "cn", Remarks: "中国大陆"},
		Domains: []shadowrocket.DomainLine{
			{Kind: model.DomainSuffix, Value: "suffix.example"},
			{Kind: model.DomainFull, Value: "full.example"},
			{Kind: model.DomainKeyword, Value: "needle"},
		},
		CIDRs: []netip.Prefix{
			netip.MustParsePrefix("192.0.2.0/24"),
			netip.MustParsePrefix("2001:db8::/32"),
		},
	}}
	rendered := mustRender(t, groups, "FlameMida/c2p-rules")
	rules := section(rendered, "[Rule]")
	golden, err := os.ReadFile(filepath.Join("testdata", "rules.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if rules != string(golden) {
		t.Fatalf("rules mismatch\nwant:\n%s\ngot:\n%s", golden, rules)
	}
}

func Test组名与顺序对齐YAML(t *testing.T) {
	groups, err := config.LoadGroups("../../config/passwall2-groups.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 16 {
		t.Fatalf("groups=%d", len(groups))
	}
	expanded := make([]shadowrocket.GroupRules, len(groups))
	for index, group := range groups {
		expanded[index] = shadowrocket.GroupRules{
			Group: group,
			Domains: []shadowrocket.DomainLine{{
				Kind: model.DomainSuffix, Value: group.ID + ".example",
			}},
		}
	}
	rendered := mustRender(t, expanded, "FlameMida/c2p-rules")
	proxyGroups := section(rendered, "[Proxy Group]")
	rules := section(rendered, "[Rule]")
	for _, group := range groups {
		if !strings.Contains(proxyGroups, group.Remarks+" = select,") {
			t.Fatalf("missing policy group %q", group.Remarks)
		}
	}
	if strings.Index(proxyGroups, "YouTube =") >= strings.Index(proxyGroups, "Google 服务 =") {
		t.Fatal("YouTube policy group does not precede Google")
	}
	if strings.Index(rules, ",YouTube\n") >= strings.Index(rules, ",Google 服务\n") {
		t.Fatal("YouTube rules do not precede Google")
	}
	if strings.Contains(rendered, "[Proxy]\n") {
		t.Fatal("renderer emitted node definitions")
	}
}

func Test广告与国内默认(t *testing.T) {
	groups := []shadowrocket.GroupRules{
		oneRule("reject", "广告拦截"),
		oneRule("youtube", "YouTube"),
		oneRule("private", "私有网络"),
		oneRule("cn", "中国大陆"),
		oneRule("direct", "直连规则"),
	}
	rendered := mustRender(t, groups, "FlameMida/c2p-rules")
	for _, want := range []string{
		"广告拦截 = select,REJECT,PROXY,DIRECT,policy-select-name=REJECT",
		"YouTube = select,PROXY,DIRECT,policy-select-name=PROXY",
		"私有网络 = select,DIRECT,PROXY,policy-select-name=DIRECT",
		"中国大陆 = select,DIRECT,PROXY,policy-select-name=DIRECT",
		"直连规则 = select,DIRECT,PROXY,policy-select-name=DIRECT",
	} {
		if !strings.Contains(rendered, want+"\n") {
			t.Fatalf("missing %q in\n%s", want, rendered)
		}
	}
}

func Test空组失败(t *testing.T) {
	_, err := shadowrocket.Render([]shadowrocket.GroupRules{{
		Group: model.Group{ID: "cn", Remarks: "中国大陆"},
	}}, "FlameMida/c2p-rules")
	if err == nil || !strings.Contains(err.Error(), "中国大陆") {
		t.Fatalf("err=%v", err)
	}
}

func TestLatestUpdateURL(t *testing.T) {
	rendered := mustRender(t, []shadowrocket.GroupRules{oneRule("cn", "中国大陆")}, "FlameMida/c2p-rules")
	want := "update-url = https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf"
	if !strings.Contains(rendered, want+"\n") {
		t.Fatalf("rendered=%s", rendered)
	}
	if strings.Contains(rendered, "release-test-tag") {
		t.Fatal("conf contains release tag")
	}
	for _, repo := range []string{"", "owner/repo", "bad repo/name", "one-part"} {
		if _, err := shadowrocket.Render([]shadowrocket.GroupRules{oneRule("cn", "中国大陆")}, repo); err == nil {
			t.Fatalf("repo %q accepted", repo)
		}
	}
}

func Test兜底顺序(t *testing.T) {
	rendered := mustRender(t, []shadowrocket.GroupRules{oneRule("cn", "中国大陆")}, "FlameMida/c2p-rules")
	lines := strings.Split(strings.TrimSpace(section(rendered, "[Rule]")), "\n")
	if len(lines) < 2 || lines[len(lines)-2] != "GEOIP,CN,中国大陆" || lines[len(lines)-1] != "FINAL,PROXY" {
		t.Fatalf("rule tail=%v", lines)
	}
	if strings.Count(rendered, "GEOIP,CN,中国大陆\n") != 1 || strings.Count(rendered, "FINAL,PROXY\n") != 1 {
		t.Fatalf("fallback count wrong:\n%s", rendered)
	}
}

func oneRule(id, remarks string) shadowrocket.GroupRules {
	return shadowrocket.GroupRules{
		Group: model.Group{ID: id, Remarks: remarks},
		Domains: []shadowrocket.DomainLine{{
			Kind: model.DomainSuffix, Value: id + ".example",
		}},
	}
}

func mustRender(t *testing.T, groups []shadowrocket.GroupRules, repo string) string {
	t.Helper()
	data, err := shadowrocket.Render(groups, repo)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func section(document, header string) string {
	start := strings.Index(document, header+"\n")
	if start < 0 {
		return ""
	}
	value := document[start+len(header)+1:]
	if end := strings.Index(value, "\n["); end >= 0 {
		value = value[:end+1]
	}
	return value
}
~~~

- [x] **步骤 2：运行测试确认失败**

~~~bash
go test ./internal/shadowrocket -run 'Test(四种可映射类型与CIDR|组名与顺序对齐YAML|广告与国内默认|空组失败|LatestUpdateURL|兜底顺序)$' -v
~~~

预期：FAIL，报 shadowrocket.Render 未定义。

- [x] **步骤 3：写最小实现**

创建 internal/shadowrocket/render.go：

~~~go
package shadowrocket

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"clash-rules-srs/internal/model"
)

var repositoryPattern = regexp.MustCompile("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")

func Render(groups []GroupRules, repo string) ([]byte, error) {
	if !repositoryPattern.MatchString(repo) || strings.EqualFold(repo, "owner/repo") {
		return nil, fmt.Errorf("invalid or placeholder repository %q", repo)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("no Shadowrocket groups")
	}

	var output bytes.Buffer
	fmt.Fprintf(&output, "[General]\nupdate-url = https://github.com/%s/releases/latest/download/shadowrocket.conf\n\n", repo)
	output.WriteString("[Proxy Group]\n")
	cnRemarks := ""
	for _, rules := range groups {
		if len(rules.Domains) == 0 && len(rules.CIDRs) == 0 {
			return nil, fmt.Errorf("group %q (%s) has zero rules", rules.Group.Remarks, rules.Group.ID)
		}
		if err := validateRemark(rules.Group.Remarks); err != nil {
			return nil, fmt.Errorf("group %q: %w", rules.Group.ID, err)
		}
		candidates, selected := policy(rules.Group.ID)
		fmt.Fprintf(&output, "%s = select,%s,policy-select-name=%s\n",
			rules.Group.Remarks, strings.Join(candidates, ","), selected)
		if rules.Group.ID == "cn" {
			if cnRemarks != "" {
				return nil, fmt.Errorf("duplicate cn group")
			}
			cnRemarks = rules.Group.Remarks
		}
	}
	if cnRemarks == "" {
		return nil, fmt.Errorf("cn group is required for GEOIP fallback")
	}

	output.WriteString("\n[Rule]\n")
	for _, rules := range groups {
		for _, line := range rules.Domains {
			if !validRuleValue(line.Value) {
				return nil, fmt.Errorf("group %q has invalid domain value %q", rules.Group.ID, line.Value)
			}
			kind, err := ruleKind(line.Kind)
			if err != nil {
				return nil, fmt.Errorf("group %q: %w", rules.Group.ID, err)
			}
			fmt.Fprintf(&output, "%s,%s,%s\n", kind, line.Value, rules.Group.Remarks)
		}
		for _, prefix := range rules.CIDRs {
			if !prefix.IsValid() {
				return nil, fmt.Errorf("group %q has invalid CIDR", rules.Group.ID)
			}
			kind := "IP-CIDR6"
			if prefix.Addr().Is4() {
				kind = "IP-CIDR"
			}
			fmt.Fprintf(&output, "%s,%s,%s,no-resolve\n", kind, prefix.Masked(), rules.Group.Remarks)
		}
	}
	fmt.Fprintf(&output, "GEOIP,CN,%s\nFINAL,PROXY\n", cnRemarks)
	return output.Bytes(), nil
}

func policy(id string) ([]string, string) {
	switch id {
	case "reject":
		return []string{"REJECT", "PROXY", "DIRECT"}, "REJECT"
	case "cn", "private", "direct":
		return []string{"DIRECT", "PROXY"}, "DIRECT"
	default:
		return []string{"PROXY", "DIRECT"}, "PROXY"
	}
}

func ruleKind(kind model.DomainKind) (string, error) {
	switch kind {
	case model.DomainSuffix:
		return "DOMAIN-SUFFIX", nil
	case model.DomainFull:
		return "DOMAIN", nil
	case model.DomainKeyword:
		return "DOMAIN-KEYWORD", nil
	default:
		return "", fmt.Errorf("unsupported domain kind %q", kind)
	}
}

func validateRemark(value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, ",=[]") {
		return fmt.Errorf("invalid remarks %q", value)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("invalid remarks %q", value)
		}
	}
	return nil
}
~~~

- [x] **步骤 4：运行测试确认通过**

~~~bash
gofmt -w internal/shadowrocket/render.go internal/shadowrocket/render_test.go
go test ./internal/shadowrocket -v
~~~

预期：任务 1 与任务 2 的 10 个测试全部 PASS；[Rule] golden 匹配；测试明确确认没有 [Proxy]。

- [x] **步骤 5：提交**

~~~bash
git add internal/shadowrocket/render.go internal/shadowrocket/render_test.go internal/shadowrocket/testdata/rules.golden
git commit -m "feat(T2): 渲染 Shadowrocket 胖配置"
~~~

## 构建与发布门禁

### 任务 3：接入事务构建并生成八资产

**文件**：
- 修改：internal/app/build.go:46-112
- 修改：internal/app/deps.go:10-22,34-240
- 修改：internal/app/build_test.go:18-294
- 修改：internal/app/build_integration_test.go:26-131,300-385
- 修改：internal/app/testdata/groups.yaml

**接口**：
- 消费：任务 1 的 shadowrocket.Expand、任务 2 的 shadowrocket.Render、现有 fileutil.AtomicWrite 与 verify.WriteSHA256。
- 产出：Dependencies.RenderShadowrocket buildStage；Dependencies.VerifyReleaseAssets buildStage；完整 build 的 staging publish/ 含 conf 与 checksum 后才 Commit。

- [x] **步骤 1：写失败测试**

在 internal/app/build_test.go 增加事务、skip-compile 与新阶段断言，并把 fixture 的正式资产由六个改为八个：

~~~go
func Test展开失败则不发布(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "render shadowrocket conf and checksum"
	fixture.failError = errors.New("group \"坏组\" references missing geosite:not-exist")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "坏组") || !strings.Contains(err.Error(), "geosite:not-exist") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}
~~~

把 TestBuildSkipCompileDoesNotSwitchOutputs 重命名为 TestSkipCompile不产出Conf，并把末尾扩为：

~~~go
for _, forbidden := range []string{
	"compile geosite",
	"render shadowrocket conf and checksum",
	"verify release assets",
} {
	if slices.Contains(fixture.calls, forbidden) {
		t.Fatalf("%s ran during skip-compile: %v", forbidden, fixture.calls)
	}
}
~~~

fixture.dependencies() 增加：

~~~go
RenderShadowrocket: stage("render shadowrocket conf and checksum", func(state *buildState) error {
	path := filepath.Join(state.Tx.Layout().Publish, "shadowrocket.conf")
	if err := os.WriteFile(path, []byte("[General]\n"), 0o600); err != nil {
		return err
	}
	_, err := verify.WriteSHA256(path)
	return err
}),
VerifyReleaseAssets: stage("verify release assets", func(state *buildState) error {
	return verify.Assets(state.Tx.Layout().Publish, verify.ReleaseAssets())
}),
~~~

并删除旧 VerifySixAssets fixture 字段。

在 internal/app/testdata/groups.yaml 末尾加入可从现有 cn community 与合成 GeoIP 底包展开的组：

~~~yaml
  - id: cn
    remarks: 中国大陆
    geosite: [cn]
    geoip: [cn]
~~~

在 internal/app/build_integration_test.go 的合成 base 中把 cn 改为双栈底包：

~~~go
"cn": "192.0.2.0/24\n2001:db8::/32\n",
~~~

把 TestSyntheticFullBuildMergesAndSeparatesApprovedTags 重命名为 Test完整构建得到八文件且Checksum匹配，并在其现有断言之后加入：

~~~go
publish := filepath.Join(root, "publish")
if err := verify.Assets(publish, verify.ReleaseAssets()); err != nil {
	t.Fatal(err)
}
confBytes, err := os.ReadFile(filepath.Join(publish, "shadowrocket.conf"))
if err != nil {
	t.Fatal(err)
}
conf := string(confBytes)
for _, want := range []string{
	"DOMAIN-SUFFIX,youtube-base.example,Google 服务",
	"IP-CIDR,192.0.2.0/24,中国大陆,no-resolve",
	"IP-CIDR6,2001:db8::/32,中国大陆,no-resolve",
} {
	if !strings.Contains(conf, want+"\n") {
		t.Fatalf("conf missing %q:\n%s", want, conf)
	}
}
checksum, err := os.ReadFile(filepath.Join(publish, "shadowrocket.conf.sha256sum"))
if err != nil || !strings.HasSuffix(string(checksum), "  shadowrocket.conf\n") {
	t.Fatalf("checksum=%q err=%v", checksum, err)
}
if strings.Index(conf, ",YouTube\n") >= strings.Index(conf, ",Google 服务\n") {
	t.Fatal("YouTube rules do not precede Google rules")
}
~~~

替换该测试内旧的六资产显式列表断言，避免同一权威清单出现两份。

- [x] **步骤 2：运行测试确认失败**

~~~bash
go test ./internal/app -run 'Test(展开失败则不发布|SkipCompile不产出Conf)$' -v
go test -tags=integration ./internal/app -run Test完整构建得到八文件且Checksum匹配 -v
~~~

预期：第一个命令因 Dependencies 缺 RenderShadowrocket / VerifyReleaseAssets 而构建失败；第二个命令因缺 shadowrocket.conf 或资产集合仍为六文件而 FAIL。

- [x] **步骤 3：写最小实现**

在 internal/app/build.go 把 Dependencies 尾部与 finish 阶段改为：

~~~go
type Dependencies struct {
	Begin               func(string) (*workspace.Transaction, error)
	LoadConfig          buildStage
	FetchBaseGeoIP      buildStage
	BuildRegistry       buildStage
	FetchRemote         buildStage
	LoadCustom          buildStage
	MergeGeoSite        buildStage
	PrepareGeoIP        buildStage
	WriteManifest       buildStage
	CompileGeoSite      buildStage
	CompileGeoIP        buildStage
	ProbeTags           buildStage
	ValidateGroups      buildStage
	RenderInstaller     buildStage
	RenderShadowrocket  buildStage
	VerifyReleaseAssets buildStage
}
~~~

~~~go
finish := []namedStage{
	{"compile geosite", dependencies.CompileGeoSite},
	{"compile geoip", dependencies.CompileGeoIP},
	{"validate passwall groups", dependencies.ValidateGroups},
	{"probe required and forbidden tags", dependencies.ProbeTags},
	{"render installer and checksums", dependencies.RenderInstaller},
	{"render shadowrocket conf and checksum", dependencies.RenderShadowrocket},
	{"verify release assets", dependencies.VerifyReleaseAssets},
}
~~~

在 internal/app/deps.go imports 加入：

~~~go
"clash-rules-srs/internal/shadowrocket"
~~~

在 ProductionDependencies 的 RenderInstaller 后加入：

~~~go
RenderShadowrocket: func(ctx context.Context, state *buildState) error {
	publish := state.Tx.Layout().Publish
	expanded, err := shadowrocket.Expand(
		ctx,
		filepath.Join(publish, "geosite.dat"),
		filepath.Join(publish, "geoip.dat"),
		state.Groups,
	)
	if err != nil {
		return err
	}
	conf, err := shadowrocket.Render(expanded, state.Options.Repo)
	if err != nil {
		return err
	}
	path := filepath.Join(publish, "shadowrocket.conf")
	if err := fileutil.AtomicWrite(path, conf, 0o644); err != nil {
		return err
	}
	_, err = verify.WriteSHA256(path)
	return err
},
VerifyReleaseAssets: func(_ context.Context, state *buildState) error {
	return verify.Assets(state.Tx.Layout().Publish, verify.ReleaseAssets())
},
~~~

删除旧 VerifySixAssets 初始化。RenderShadowrocket 的任何错误由 Build defer Abort staging；Commit 之前不会切换 build/ 或 publish/。

- [x] **步骤 4：运行测试确认通过**

~~~bash
gofmt -w internal/app/build.go internal/app/deps.go internal/app/build_test.go internal/app/build_integration_test.go
go test ./internal/app -run 'Test(展开失败则不发布|SkipCompile不产出Conf|BuildEmitsAndProbesEveryOutput)$' -v
go test -tags=integration ./internal/app -run 'Test(完整构建得到八文件且Checksum匹配|ProductionDependenciesFailuresPreservePreviousGeneration)$' -v
~~~

预期：全部 PASS；真实工具集成测试证明 Google 组含 youtube include 域名、cn 含两个底包前缀、checksum 格式正确、失败仍保留 old generation。

- [x] **步骤 5：提交**

~~~bash
git add internal/app/build.go internal/app/deps.go internal/app/build_test.go internal/app/build_integration_test.go internal/app/testdata/groups.yaml
git commit -m "feat(T3): 将 Shadowrocket 配置接入事务构建"
~~~

### 任务 4：升级资产闭集与发布指纹

**文件**：
- 修改：internal/verify/assets.go:16-29
- 修改：internal/verify/assets_test.go:13-91
- 修改：internal/releasecmp/releasecmp.go:104-132
- 修改：internal/releasecmp/releasecmp_test.go:17-284
- 修改：internal/app/commands_test.go:86-161

**接口**：
- 消费：任务 3 写出的 shadowrocket.conf 与 shadowrocket.conf.sha256sum。
- 产出：verify.ReleaseAssets() 返回独立的精确八名切片；releasecmp fingerprint 第四帧为 shadowrocket.conf 原始字节，不读取其 checksum 内容。

- [x] **步骤 1：写失败测试**

把 internal/verify/assets_test.go 的资产期望统一改名为 releaseAssets，把 TestWriteSHA256AndValidateExactSixAssets 重命名为 TestWriteSHA256AndValidateExactEightAssets，并加入两个新文件：

~~~go
var releaseAssets = []string{
	"geoip.dat",
	"geoip.dat.sha256sum",
	"geosite.dat",
	"geosite.dat.sha256sum",
	"install_passwall2_rules.sh",
	"install_passwall2_rules.sh.sha256sum",
	"shadowrocket.conf",
	"shadowrocket.conf.sha256sum",
}

func TestReleaseAssetsReturnsIndependentExactEightNames(t *testing.T) {
	first := verify.ReleaseAssets()
	if !slices.Equal(first, releaseAssets) {
		t.Fatalf("assets=%v", first)
	}
	first[0] = "mutated"
	if got := verify.ReleaseAssets(); !slices.Equal(got, releaseAssets) {
		t.Fatalf("shared mutable assets=%v", got)
	}
}

func Test缺少Conf时资产校验失败(t *testing.T) {
	dir := validAssets(t)
	if err := os.Remove(filepath.Join(dir, "shadowrocket.conf")); err != nil {
		t.Fatal(err)
	}
	err := verify.Assets(dir, verify.ReleaseAssets())
	if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "shadowrocket.conf") {
		t.Fatalf("err=%v", err)
	}
}
~~~

validAssets 与 checksum 测试的主文件循环改为：

~~~go
for _, name := range []string{
	"geosite.dat",
	"geoip.dat",
	"install_passwall2_rules.sh",
	"shadowrocket.conf",
} {
	path := filepath.Join(dir, name)
	mustWrite(t, path, name)
	if _, err := verify.WriteSHA256(path); err != nil {
		t.Fatal(err)
	}
}
~~~

把 internal/releasecmp/releasecmp_test.go 的 TestLatest六资产损坏重命名为 TestLatest八资产损坏，再增加：

~~~go
func Test只改Conf判定Changed(t *testing.T) {
	candidate := payload(t, "candidate", "same-site", "same-ip", "logic=v1")
	baseline := payload(t, "baseline", "same-site", "same-ip", "logic=v1")
	rewriteMain(t, candidate, "shadowrocket.conf", "conf=v2")
	decision, err := releasecmp.Decide(compareInput(candidate, "candidate", baseline, "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.ShouldPublish || decision.Reason != "changed" {
		t.Fatalf("decision=%+v", decision)
	}
}

func Test只换Tag不改Conf判定Unchanged(t *testing.T) {
	candidate := payload(t, "candidate", "same-site", "same-ip", "logic=v1")
	baseline := payload(t, "baseline", "same-site", "same-ip", "logic=v1")
	decision, err := releasecmp.Decide(compareInput(candidate, "candidate", baseline, "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	if decision.ShouldPublish || decision.Reason != "unchanged" {
		t.Fatalf("decision=%+v", decision)
	}
}

func Test六文件基线不得当成Changed(t *testing.T) {
	candidate := payload(t, "candidate", "site", "ip", "logic=v1")
	baseline := payload(t, "baseline", "site", "ip", "logic=v1")
	for _, name := range []string{"shadowrocket.conf", "shadowrocket.conf.sha256sum"} {
		if err := os.Remove(filepath.Join(baseline, name)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := releasecmp.Decide(compareInput(candidate, "candidate", baseline, "baseline"))
	if err == nil || !strings.Contains(err.Error(), "baseline payload") ||
		!strings.Contains(err.Error(), "asset set mismatch") ||
		!strings.Contains(err.Error(), "missing") {
		t.Fatalf("err=%v", err)
	}
}
~~~

payload 与 renderedPayload helper 在写完安装器后各增加：

~~~go
rewriteMain(t, dir, "shadowrocket.conf", "conf=v1")
~~~

在 internal/app/commands_test.go 的 releasePayload helper 同样写 conf，并增加命令级无 stdout 断言：

~~~go
func Test六文件基线不得当成ChangedCLI(t *testing.T) {
	candidate := releasePayload(t, "candidate", "site", "ip")
	baseline := releasePayload(t, "baseline", "site", "ip")
	for _, name := range []string{"shadowrocket.conf", "shadowrocket.conf.sha256sum"} {
		if err := os.Remove(filepath.Join(baseline, name)); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := runReleaseDecision([]string{
		"--candidate", candidate,
		"--candidate-tag", "candidate",
		"--baseline", baseline,
		"--baseline-tag", "baseline",
	})
	if code != 1 || stdout != "" ||
		!strings.Contains(stderr, "asset set mismatch") ||
		!strings.Contains(stderr, "missing") ||
		strings.Contains(stdout, "reason=changed") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
~~~

- [x] **步骤 2：运行测试确认失败**

~~~bash
go test ./internal/verify -run 'Test(ReleaseAssetsReturnsIndependentExactEightNames|缺少Conf时资产校验失败)$' -v
go test ./internal/releasecmp -run 'Test(只改Conf判定Changed|只换Tag不改Conf判定Unchanged|六文件基线不得当成Changed)$' -v
go test ./internal/app -run Test六文件基线不得当成ChangedCLI -v
~~~

预期：verify 因仍返回六名而 FAIL；releasecmp / app helper 在资产验证或 conf 指纹断言处 FAIL。

- [x] **步骤 3：写最小实现**

把 internal/verify/assets.go 的 releaseAssetNames 改为：

~~~go
var releaseAssetNames = [...]string{
	"geoip.dat",
	"geoip.dat.sha256sum",
	"geosite.dat",
	"geosite.dat.sha256sum",
	"install_passwall2_rules.sh",
	"install_passwall2_rules.sh.sha256sum",
	"shadowrocket.conf",
	"shadowrocket.conf.sha256sum",
}
~~~

在 internal/releasecmp/releasecmp.go 的 fingerprint 中，保留两个 dat 与规范化 installer 帧；installer 帧完成后、返回 digest 前追加：

~~~go
if err := hashFileFrame(digest, directory, "shadowrocket.conf"); err != nil {
	return "", err
}
return hex.EncodeToString(digest.Sum(nil)), nil
~~~

不得 hash shadowrocket.conf.sha256sum；verify.Assets 已先校验该文件和主文件匹配。

- [x] **步骤 4：运行测试确认通过**

~~~bash
gofmt -w internal/verify/assets.go internal/verify/assets_test.go internal/releasecmp/releasecmp.go internal/releasecmp/releasecmp_test.go internal/app/commands_test.go
go test ./internal/verify ./internal/releasecmp ./internal/app
~~~

预期：全部 PASS；conf 单独变化为 changed；只有安装器 tag 变化为 unchanged；旧六文件 baseline 返回资产集合错误而不是 changed。

- [x] **步骤 5：提交**

~~~bash
git add internal/verify/assets.go internal/verify/assets_test.go internal/releasecmp/releasecmp.go internal/releasecmp/releasecmp_test.go internal/app/commands_test.go
git commit -m "feat(T4): 将 Shadowrocket 资产纳入发布门禁"
~~~

### 任务 5：升级 GitHub Workflow 八资产事务

**文件**：
- 修改：internal/app/workflow_contract_test.go:171-212,248-276
- 修改：.github/workflows/build.yml:65-71,144-156,180-195,304-356

**接口**：
- 消费：任务 4 的八资产闭集与 release-decision。
- 产出：build Artifact、publish exact check、draft upload、API 资产名回读、下载回读和四份 checksum 校验均使用同一八资产集合。

- [ ] **步骤 1：写失败测试**

把 internal/app/workflow_contract_test.go 的草稿事务测试改名为 Test变化后草稿Release八资产回读契约，并将精确 upload 期望改为：

~~~go
upload := "gh release upload \"$TAG\" \\\n" +
	"  publish/geoip.dat \\\n" +
	"  publish/geoip.dat.sha256sum \\\n" +
	"  publish/geosite.dat \\\n" +
	"  publish/geosite.dat.sha256sum \\\n" +
	"  publish/install_passwall2_rules.sh \\\n" +
	"  publish/install_passwall2_rules.sh.sha256sum \\\n" +
	"  publish/shadowrocket.conf \\\n" +
	"  publish/shadowrocket.conf.sha256sum"
if !strings.Contains(stage.Run, upload) {
	t.Fatalf("release upload is not exact eight assets:\n%s", stage.Run)
}
~~~

在该测试的有序 fragment 中，把 checksum 回读扩为：

~~~go
"gh release download \"$TAG\"",
"sha256sum -c geoip.dat.sha256sum",
"sha256sum -c geosite.dat.sha256sum",
"sha256sum -c install_passwall2_rules.sh.sha256sum",
"sha256sum -c shadowrocket.conf.sha256sum",
~~~

并新增一个针对 build Artifact 和 publish exact check 的独立测试：

~~~go
func TestWorkflow上传与校验八资产(t *testing.T) {
	document := loadWorkflow(t)
	build := document.Jobs["build"]
	upload := stepByUses(t, build, "actions/upload-artifact@")
	path, ok := upload.With["path"].(string)
	if !ok {
		t.Fatalf("artifact path=%#v", upload.With["path"])
	}
	for _, name := range []string{
		"geoip.dat", "geoip.dat.sha256sum",
		"geosite.dat", "geosite.dat.sha256sum",
		"install_passwall2_rules.sh", "install_passwall2_rules.sh.sha256sum",
		"shadowrocket.conf", "shadowrocket.conf.sha256sum",
	} {
		if strings.Count(path, "publish/"+name) != 1 {
			t.Fatalf("artifact path count for %s:\n%s", name, path)
		}
	}
	for _, step := range []workflowStep{
		stepByName(t, document.Jobs["publish"], "Verify exact payload and checksums"),
		stepByName(t, document.Jobs["publish"], "Stage, read back, and publish exact Release"),
	} {
		assertRunContains(t, step, "shadowrocket.conf \\", "shadowrocket.conf.sha256sum")
	}
	for _, step := range []workflowStep{
		stepByName(t, build, "Verify all checksums"),
		stepByName(t, document.Jobs["publish"], "Verify exact payload and checksums"),
		stepByName(t, document.Jobs["publish"], "Stage, read back, and publish exact Release"),
	} {
		if !strings.Contains(step.Run, "sha256sum -c shadowrocket.conf.sha256sum") {
			t.Fatalf("%s misses Shadowrocket checksum:\n%s", step.Name, step.Run)
		}
	}
}
~~~

- [ ] **步骤 2：运行测试确认失败**

~~~bash
go test ./internal/app -run 'Test(变化后草稿Release八资产回读契约|Workflow上传与校验八资产)$' -v
~~~

预期：FAIL，指出 Artifact path、checksum 或 gh release upload 缺 shadowrocket.conf 两个资产。

- [ ] **步骤 3：写最小实现**

在 .github/workflows/build.yml 做以下精确替换：

~~~yaml
- name: Verify all checksums
  run: |
    set -euo pipefail
    cd publish
    sha256sum -c geoip.dat.sha256sum
    sha256sum -c geosite.dat.sha256sum
    sha256sum -c install_passwall2_rules.sh.sha256sum
    sha256sum -c shadowrocket.conf.sha256sum
~~~

~~~yaml
- name: Upload verified eight-asset payload
  if: steps.release_decision.outputs.should_publish == 'true'
  uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02
  with:
    name: verified-geodata
    path: |
      publish/geoip.dat
      publish/geoip.dat.sha256sum
      publish/geosite.dat
      publish/geosite.dat.sha256sum
      publish/install_passwall2_rules.sh
      publish/install_passwall2_rules.sh.sha256sum
      publish/shadowrocket.conf
      publish/shadowrocket.conf.sha256sum
    if-no-files-found: error
    retention-days: 1
~~~

publish job 的 expected 清单、gh release upload 参数和 readback checksum 均在安装器两行后追加：

~~~bash
shadowrocket.conf
shadowrocket.conf.sha256sum
~~~

~~~bash
publish/shadowrocket.conf \
publish/shadowrocket.conf.sha256sum
~~~

~~~bash
sha256sum -c shadowrocket.conf.sha256sum
~~~

expected 仍通过 sort 与 actual 精确相等；不得只检查“至少存在”。

- [ ] **步骤 4：运行测试确认通过**

~~~bash
go test ./internal/app -run 'Test(变化后草稿Release八资产回读契约|Workflow上传与校验八资产|干净Runner完成无变化判定契约|WorkflowPermissionsOutputsAndConcurrency)$' -v
~~~

预期：全部 PASS；Workflow YAML 可解析；八个名字在 Artifact 与 Release upload 中各出现一次，四份 checksum 在构建和回读阶段均校验。

- [ ] **步骤 5：提交**

~~~bash
git add internal/app/workflow_contract_test.go .github/workflows/build.yml
git commit -m "ci(T5): 发布并回读 Shadowrocket 八资产"
~~~

## 文档与验收

### 任务 6：记录导入、更新与首次强制发布

**文件**：
- 修改：internal/app/docs_contract_test.go:10-64
- 修改：README.md:1-16,70-100,126-138
- 修改：context.md:3-30,60-90

**接口**：
- 消费：任务 3 的 shadowrocket.conf latest URL 与任务 5 的 force_publish 工作流入口。
- 产出：README.md 与 context.md 均列出八资产，并提供 HTTPS 粘贴、同 URL 二维码、URL-encoded scheme 三种入口以及首次上线操作。

- [ ] **步骤 1：写失败测试**

在 internal/app/docs_contract_test.go 现有双文档循环的 required 列表增加：

~~~go
"shadowrocket.conf",
"shadowrocket.conf.sha256sum",
"https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf",
"二维码",
"shadowrocket://config/add/",
"force_publish=true",
"八资产",
~~~

在 forbidden 列表增加所有旧发布口径：

~~~go
"精确六个文件",
"候选六资产",
"规范化三主资产",
"六资产验证",
"精确比较六个文件",
~~~

并新增 Scenario 对应测试，明确两份文档分别独立满足：

~~~go
func Test文档含新资产与导入入口(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range []string{"README.md", "context.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, want := range []string{
			"shadowrocket.conf.sha256sum",
			"force_publish=true",
			"shadowrocket://config/add/",
			"https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf",
			"二维码",
			"镜像前缀",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
		for _, old := range []string{"精确六个文件", "规范化三主资产"} {
			if strings.Contains(text, old) {
				t.Errorf("%s keeps stale release wording %q", name, old)
			}
		}
	}
}
~~~

为此在 test imports 加入 strings。

- [ ] **步骤 2：运行测试确认失败**

~~~bash
go test ./internal/app -run 'Test(DocsDescribeGoOnlyWorkflowAndManagedInstall|文档含新资产与导入入口)$' -v
~~~

预期：FAIL，README.md 与 context.md 缺 Shadowrocket 资产、scheme、二维码和 force_publish=true，并仍含六资产 / 三主资产旧口径。

- [ ] **步骤 3：写最小实现**

把 README.md 的发布清单改成：

~~~~markdown
每次 Release 精确包含八资产：

~~~text
geoip.dat
geoip.dat.sha256sum
geosite.dat
geosite.dat.sha256sum
install_passwall2_rules.sh
install_passwall2_rules.sh.sha256sum
shadowrocket.conf
shadowrocket.conf.sha256sum
~~~
~~~~

在 README.md 的本地构建段之后加入以下完整章节：

~~~markdown
## 导入 Shadowrocket

Shadowrocket 配置使用同一套最终 GeoSite/GeoIP 数据展开出的胖配置，不含节点；导入前需先在 App 中添加可供 PROXY 使用的订阅或节点。必须支持的入口是在 Shadowrocket 配置页粘贴：

https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf

也可以把同一 HTTPS URL 交给可信的本地或在线二维码生成器，再用 Shadowrocket 扫码；仓库不发布二维码图片。作为增强入口，可打开 URL-encoded scheme：

shadowrocket://config/add/https%3A%2F%2Fgithub.com%2FFlameMida%2Fc2p-rules%2Freleases%2Flatest%2Fdownload%2Fshadowrocket.conf

配置内的 update-url 始终是上述 GitHub 官方 latest URL，不写 gh-proxy.com、ghfast.top 等镜像前缀。自动更新会覆盖配置正文；App 是否保留组内当前选择不属于本仓契约。

发布集合从旧版切换为八资产后，旧 latest 无法通过严格 baseline 比较。首次上线必须在默认分支手动运行 build-geodata Workflow，并设置 force_publish=true；此后普通构建会把 shadowrocket.conf 原始字节纳入变化判定。
~~~

把 README.md 其余“六资产”“三主资产”发布口径逐处改成“八资产”“两个 dat、规范化安装器与 shadowrocket.conf 四个主资产”，并把 checksum 数量改为四份。

把 context.md 的当前交付与数据流改为：

~~~markdown
数据面统一使用 geosite.dat / geoip.dat；Release 固定为两个 dat、安装器、Shadowrocket 胖配置及四份对应 checksum，共八资产。

sources.yaml + custom/geosite + custom/geoip
  -> 严格配置与 Clash 规则解析
  -> 显式 create / merge-base 目标注册
  -> community GeoSite 精确去重合并
  -> base GeoIP + 规范化 CIDR 输入合并
  -> pinned domain-list-custom / geoip
  -> geoview required + forbidden 探针
  -> config/passwall2-groups.yaml 引用验证
  -> 从最终 dat 展开并渲染 shadowrocket.conf
  -> 八资产 checksum 与事务性发布
~~~

在 context.md 分流章节后加入与 README 相同的 actual latest URL、二维码说明、完整 URL-encoded scheme、官方 update-url 不带镜像前缀、自动更新覆盖边界与默认分支 force_publish=true 首次上线步骤。CI 章节统一改成“四个主资产”“八资产验证”“精确比较八个文件”“四份 checksum”；验收边界补充真机导入、自动更新和大 conf 耗时为 DEFERRED。

- [ ] **步骤 4：运行测试确认通过**

~~~bash
gofmt -w internal/app/docs_contract_test.go
go test ./internal/app -run 'Test(DocsDescribeGoOnlyWorkflowAndManagedInstall|文档含新资产与导入入口)$' -v
~~~

预期：两个测试 PASS；README.md 与 context.md 均含实际 latest URL、三种入口、镜像禁入 update-url、force_publish=true，且不再出现旧六资产 / 三主资产口径。

- [ ] **步骤 5：提交**

~~~bash
git add internal/app/docs_contract_test.go README.md context.md
git commit -m "docs(T6): 说明 Shadowrocket 导入与首次发布"
~~~

### 任务 7：验收（acceptance-qa）

> 本任务由 executing-plans 收尾审查阶段触发 acceptance-qa 按下表执行，不参与逐任务连续执行；报告与证据落盘特性目录 acceptance/ 子目录。

| Scenario / 检查项 | 维度 | 执行方式 | 目标 | 阈值/预期 | 验收证据 |
|-------------------|------|---------|------|----------|---------|
| 真机导入与自动更新 | e2e | 验收任务 (D) | https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf 与已授权 Shadowrocket 真机 | DEFERRED；只有在八资产 Release 已发布且获得真机授权后，才执行 HTTPS 导入、scheme 导入、策略组可见性及下一版 conf 自动更新验证；本地不得标记 PASS | .spec-dev/2026-08-15-shadowrocket-conf/acceptance/shadowrocket-device.md，记录 Release tag、设备/App 版本、每个入口结果、自动更新前后可观察差异或 DEFERRED 原因 |

### 任务 8：合并与清理

**资源台账**（清理依据；执行中创建持久资源时即追加；行格式为复选框、类型、标识和清理命令）：

- [ ] worktree: .worktrees/plan-2026-08-15-shadowrocket-conf —— git worktree remove .worktrees/plan-2026-08-15-shadowrocket-conf && git branch -d plan/2026-08-15-shadowrocket-conf

台账总则：**清理只遍历本台账、台账外一律不动**（可疑残留只报告不删）；共享缓存（~/.cargo、pnpm store、npm cache、Go module cache 等）默认保留，仅用户显式要求清理时才登记入账；台账限定持久资源（容器、测试库/表、临时目录、后台服务），worktree 内构建产物随 worktree 删除自然回收、不入账。

- [ ] **步骤 1：全量验证（安全网）与归属裁决**

在 worktree 内运行完整测试套件：

~~~bash
go test ./...
go test -tags=integration ./internal/app
~~~

- 全绿 → 进入步骤 2。
- 失败测试在相关测试范围内 → 修复并复跑全绿后进入步骤 2。
- 失败测试在范围之外 → 在主工作区来源分支复跑该测试；主工作区有未提交改动时先询问用户。来源分支同样失败则报告“既有失败”并请用户裁决是否阻塞合并；来源分支通过则判定为本次回归，修复并复跑全绿。

- [ ] **步骤 2：测试退役检查**

扫描相关范围 internal/app、internal/verify、internal/releasecmp、internal/shadowrocket 的测试名。只把同时满足以下两项的测试列为候选：

1. 名称对不上任何 active spec 的现行 Scenario，且属于本计划采用的 Scenario 直译命名；
2. 对应 Requirement 已 REMOVED / Superseded，或所属 spec 已 superseded。

候选非空则列清单征询用户，同意后才删除并计入本任务提交；无候选则声明“无孤儿测试”。历史上不遵循 Scenario 命名约定的测试保守豁免。

- [ ] **步骤 3：取代回写**

本 spec 的 supersedes 为空数组，声明“无取代回写”后跳过。

- [ ] **步骤 4：合并回来源分支**

~~~bash
cd "$(dirname "$(git rev-parse --git-common-dir)")"
git merge plan/2026-08-15-shadowrocket-conf
~~~

合并冲突、或主工作区有未提交改动 → 停下向计划作者确认，不强行合并。

- [ ] **步骤 5：清理（按资源台账逐条执行）**

逐条执行资源台账各行的清理命令并勾选。命令失败则该行保持未勾选并报告；资源已不存在则勾选并注明“已不存在”。台账外文件、容器和数据一律不动。

- [ ] **步骤 6：sync_commit 锚定**

~~~bash
SYNC=$(git rev-parse HEAD)
SHORT_SYNC=$(git rev-parse --short=7 "$SYNC")
# 将 spec frontmatter 的 sync_commit: null 更新为上面的完整 $SYNC
git add .spec-dev/2026-08-15-shadowrocket-conf/spec/shadowrocket-conf-design.md
git commit -m "chore(spec): sync_commit 锚定 $SHORT_SYNC"
~~~

此后 git diff <sync_commit>..HEAD -- <covers glob> 即“spec 上次确认同步以来的代码变化”。

任务 0 未由本计划建立 worktree（此前已在隔离环境、原生工具建立、或降级原地执行）时，只执行步骤 1、2、3 与步骤 6；步骤 4-5 交回原有隔离机制收尾并注明。
