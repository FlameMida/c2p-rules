# Geodata 有效变化增量发布实施计划

> **执行方式**：使用 spec-dev 的 executing-plans skill 逐任务执行本计划；无该 skill 的环境直接从任务 0 起按序执行至最终任务。步骤用复选框（`- [ ]`）语法跟踪；脱离项目携带时连同特性目录（含 spec）整体带走。
>
> **偏差处理**：执行中发现计划与现实不符——小偏差（路径笔误、明显遗漏但意图清楚）就地修正并在提交信息中注明；接口、数据结构等契约级偏差停下向计划作者确认，不猜着改。

**目标**：每日完整构建候选 geodata，但只有规范化三主资产发生有效变化、首次发布或人工强制时才创建严格六资产 Release。

**Spec**：`.spec-dev/2026-08-02-incremental-geodata-release/spec/incremental-geodata-release-design.md`

**架构**：在现有 Go CLI 内新增纯本地 `release-decision` 用例；`internal/releasecmp` 复用统一的六资产与安装器 tag 契约，先严格验证目录，再流式计算规范化载荷指纹。GitHub Actions 的只读 build job 负责完整构建、latest 状态分类和判定，具备写权限的 publish job 在任何远端写入前以同一 Go 比较器复核基线 ID、tag、指纹。

**技术栈**：Go 1.26、标准库 `crypto/sha256`/`encoding/binary`/`flag`、`go.yaml.in/yaml/v3`、GitHub Actions、GitHub CLI。

## 全局约束

- Release 资产始终精确为 `geosite.dat`、`geosite.dat.sha256sum`、`geoip.dat`、`geoip.dat.sha256sum`、`install_passwall2_rules.sh`、`install_passwall2_rules.sh.sha256sum`，不得增加第七资产。
- 发布等价键只由 `geosite.dat` 原始字节、`geoip.dat` 原始字节和仅规范化唯一合法 `RELEASE_TAG='...'` 后的完整安装器字节组成；三个 checksum 必须验证，但不独立参与比较。
- 载荷指纹使用 SHA-256，并按固定顺序把资产名长度、资产名、内容长度、内容写入；dat 必须流式读取。
- `--baseline DIR`、`--first-release`、`--force` 必须恰选一种；参数形状错误 exit 2，目录、资产、checksum 或安装器完整性错误 exit 1。
- latest 只有明确 404 才是首次发布；非 200/404、网络/鉴权/下载失败及不可信基线一律严格失败。
- `force_publish=true` 只跳过旧基线下载和内容比较，不跳过候选完整构建、严格验证、Release 回读或默认分支限制。
- build job 保持 `contents: read`；publish job 才使用 `contents: write`，且非默认分支永不创建远端 tag 或 Release。
- 固定工具 commit 不随滚动上游自动更新；生产链不得增加 Python、Node、npm 或仓库内 Shell 脚本。
- 执行时只暂存各任务列出的文件；主工作区若出现无关未提交改动，不得覆盖、代为提交或丢弃。

## 文件结构与职责

| 文件 | 变化 | 单一职责 |
|---|---|---|
| `internal/verify/assets.go` | 修改 | 导出不可变副本形式的严格六资产名称，并继续负责集合/checksum 验证 |
| `internal/passwall/installer.go` | 修改 | 导出与生成器完全一致的 Release tag 校验 |
| `internal/releasecmp/releasecmp.go` | 创建 | 安装器规范化、无歧义流式指纹、三模式决策 |
| `internal/releasecmp/releasecmp_test.go` | 创建 | 规范化载荷、坏资产、first/force 的表驱动测试 |
| `internal/app/build.go`、`internal/app/deps.go` 及相关测试 | 修改 | 构建链改用统一六资产契约，删除私有重复列表 |
| `internal/app/release_decision.go` | 创建 | 把 CLI options 映射到比较器并输出稳定三行 |
| `internal/app/commands.go`、`internal/app/commands_test.go` | 修改 | 解析恰选一种模式并接入用例 |
| `internal/cli/run.go`、`internal/cli/run_test.go` | 修改 | 注册 `release-decision` 子命令、help 和退出码 |
| `.github/workflows/build.yml` | 修改 | latest 分类、job outputs、条件 Artifact、发布前完整基线复核 |
| `internal/app/workflow_contract_test.go` | 修改 | 用 YAML job/step 结构验证权限、条件、归属和顺序 |
| `internal/app/docs_contract_test.go` | 修改 | 守卫文档中的每日检测、跳过发布和强制修复说明 |
| `README.md`、`context.md` | 修改 | 记录增量发布行为、CLI 与严格错误边界 |

---

### 任务 0：建立隔离工作区

- [x] **步骤 1：检测已有隔离**

运行：`git rev-parse --git-dir` 与 `git rev-parse --git-common-dir`
两者不同、且 `git rev-parse --show-superproject-working-tree` 无输出（排除 submodule）
→ 已在隔离工作区，跳过本任务。

- [x] **步骤 2：建立 worktree**

有原生 worktree 工具或 using-git-worktrees skill 时优先使用（Codex 无原生 worktree 工具，直接走下面的手工路径）；否则手工降级：
确认 `.worktrees/` 已被忽略（`git check-ignore -q .worktrees`，未忽略先加入 `.gitignore` 并只提交该忽略规则），然后运行：

```bash
git worktree add .worktrees/plan-2026-08-02-incremental-geodata-release -b plan/2026-08-02-incremental-geodata-release
cd .worktrees/plan-2026-08-02-incremental-geodata-release
```

worktree 从已提交的当前 `HEAD` 建立，不携带主工作区之后可能出现的未提交改动。

- [x] **步骤 3：安装依赖并验证基线**

运行：

```bash
go mod download
go test ./...
go vet ./...
```

预期：全部 exit 0。任一基线失败则停下报告，不把既有失败归因于本特性。

### 任务 1：规范化载荷比较器与统一资产契约

**文件**：
- 创建：`internal/releasecmp/releasecmp.go`
- 创建：`internal/releasecmp/releasecmp_test.go`
- 修改：`internal/verify/assets.go`
- 修改：`internal/verify/assets_test.go`
- 修改：`internal/passwall/installer.go`
- 修改：`internal/passwall/installer_test.go`
- 修改：`internal/app/build.go`
- 修改：`internal/app/deps.go`
- 修改：`internal/app/build_test.go`

**接口**：
- 消费：`verify.Assets(directory string, expected []string) error`、`verify.WriteSHA256(path string) (string, error)`。
- 产出：`verify.ReleaseAssets() []string`、`passwall.ValidateReleaseTag(tag string) error`、`releasecmp.Mode`（`Compare`/`FirstRelease`/`Force`）、`releasecmp.Input`、`releasecmp.Decision`、`releasecmp.Decide(Input) (Decision, error)`、`releasecmp.NormalizeInstaller([]byte) ([]byte, error)`。

- [x] **步骤 1：写失败测试**

先在 `internal/verify/assets_test.go` 增加一个防止调用方篡改全局契约的测试：

```go
func TestReleaseAssetsReturnsIndependentExactSixNames(t *testing.T) {
	want := []string{
		"geoip.dat", "geoip.dat.sha256sum",
		"geosite.dat", "geosite.dat.sha256sum",
		"install_passwall2_rules.sh", "install_passwall2_rules.sh.sha256sum",
	}
	first := verify.ReleaseAssets()
	if !slices.Equal(first, want) {
		t.Fatalf("assets=%v", first)
	}
	first[0] = "mutated"
	if got := verify.ReleaseAssets(); !slices.Equal(got, want) {
		t.Fatalf("shared mutable assets=%v", got)
	}
}
```

在 `internal/passwall/installer_test.go` 增加 `TestValidateReleaseTagMatchesRendererContract`，以 `release-1_A.b` 成功、空串/空格/前导连字符/129 字节失败，并确认 `RenderInstaller` 与导出校验器对同一 tag 给出一致结果。

创建 `internal/releasecmp/releasecmp_test.go`。测试辅助函数必须写出两个 dat、包含唯一 tag 行的安装器，再对三个主资产调用 `verify.WriteSHA256`，从而形成真实严格六资产目录；修改主资产后必须重写对应 checksum，确保测试区分“checksum 损坏”和“规范化比较”。至少加入以下测试，测试名直接对应 spec Scenario：

```go
func Test相同内容重复运行(t *testing.T)              // 两目录只使用不同合法 tag，断言 unchanged/false 和 64 位 baseline 指纹
func Test任一Dat发生变化(t *testing.T)               // geoip、geosite 两个子测试分别断言 changed/true
func Test安装器逻辑发生变化(t *testing.T)             // tag 外任一完整字节变化断言 changed/true
func Test安装器仅ReleaseTag不同(t *testing.T)          // NormalizeInstaller 结果逐字节相等
func Test安装器发布绑定字段异常(t *testing.T)           // 缺失、重复、空值、空格、前导连字符、超长均报错
func Test内容相同但人工强制发布(t *testing.T)           // 有效候选断言 forced/true/空 baseline 指纹
func TestLatest六资产损坏(t *testing.T)               // 基线缺失、额外、坏 checksum、坏 installer 均报错
func Test首次或强制模式下候选损坏(t *testing.T)         // first/force × 缺失/额外/坏 checksum/坏 installer 均报错
func Test尚无LatestRelease(t *testing.T)              // FirstRelease 有效候选断言 first-release/true
```

使用固定的十六进制指纹正则 `^[0-9a-f]{64}$`，另加 `TestFingerprintFramesNamesAndLengths`：交换两个 dat 的内容仍产生不同指纹，证明不是无分隔裸拼接。错误断言使用稳定的错误类别片段（`candidate`、`baseline`、`asset set mismatch`、`checksum mismatch`、`RELEASE_TAG`），不绑定临时目录绝对路径。

最后修改 `internal/app/build_test.go`，让现有构建 fixture 用 `verify.ReleaseAssets()` 验证；测试应先因 `releaseAssets` 删除目标尚未实施或新 API 不存在而无法编译。

- [x] **步骤 2：运行测试确认失败**

运行：

```bash
go test ./internal/verify ./internal/passwall ./internal/releasecmp ./internal/app
```

预期：FAIL；首个失败为 `undefined: verify.ReleaseAssets`、`undefined: passwall.ValidateReleaseTag` 或 `internal/releasecmp` 尚无 Go 文件。保留失败输出作为 TDD 证据。

- [x] **步骤 3：写最小实现**

在 `internal/verify/assets.go` 把私有数组集中为以下接口；每次返回副本：

```go
var releaseAssetNames = [...]string{
	"geoip.dat",
	"geoip.dat.sha256sum",
	"geosite.dat",
	"geosite.dat.sha256sum",
	"install_passwall2_rules.sh",
	"install_passwall2_rules.sh.sha256sum",
}

func ReleaseAssets() []string {
	assets := make([]string, len(releaseAssetNames))
	copy(assets, releaseAssetNames[:])
	return assets
}
```

在 `internal/passwall/installer.go` 增加并让 `RenderInstaller` 调用：

```go
func ValidateReleaseTag(tag string) error {
	if !releaseTagPattern.MatchString(tag) {
		return fmt.Errorf("invalid release tag %q", tag)
	}
	return nil
}
```

创建 `internal/releasecmp/releasecmp.go`，使用以下完整类型与算法边界：

```go
package releasecmp

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"clash-rules-srs/internal/passwall"
	"clash-rules-srs/internal/verify"
)

type Mode uint8

const (
	Compare Mode = iota + 1
	FirstRelease
	Force
)

type Input struct {
	CandidateDir string
	BaselineDir  string
	Mode         Mode
}

type Decision struct {
	ShouldPublish       bool
	Reason              string
	BaselineFingerprint string
}

var (
	releaseTagField      = regexp.MustCompile(`(?m)^[\t ]*RELEASE_TAG[\t ]*=.*$`)
	releaseTagAssignment = regexp.MustCompile(`^RELEASE_TAG='([^'\r\n]*)'$`)
)

const normalizedTagAssignment = "RELEASE_TAG='__CLASH_RULES_SRS_RELEASE_TAG__'"

func NormalizeInstaller(data []byte) ([]byte, error) {
	fields := releaseTagField.FindAllIndex(data, -1)
	if len(fields) != 1 {
		return nil, fmt.Errorf("installer must contain exactly one RELEASE_TAG field: found %d", len(fields))
	}
	start, end := fields[0][0], fields[0][1]
	assignment := releaseTagAssignment.FindSubmatchIndex(data[start:end])
	if assignment == nil {
		return nil, fmt.Errorf("installer RELEASE_TAG field does not use generated syntax")
	}
	value := string(data[start+assignment[2] : start+assignment[3]])
	if err := passwall.ValidateReleaseTag(value); err != nil {
		return nil, fmt.Errorf("invalid RELEASE_TAG assignment: %w", err)
	}
	result := make([]byte, 0, len(data)-(end-start)+len(normalizedTagAssignment))
	result = append(result, data[:start]...)
	result = append(result, normalizedTagAssignment...)
	result = append(result, data[end:]...)
	return result, nil
}

func Decide(input Input) (Decision, error) {
	candidateFingerprint, err := fingerprint(input.CandidateDir)
	if err != nil {
		return Decision{}, fmt.Errorf("candidate payload: %w", err)
	}
	switch input.Mode {
	case FirstRelease:
		return Decision{ShouldPublish: true, Reason: "first-release"}, nil
	case Force:
		return Decision{ShouldPublish: true, Reason: "forced"}, nil
	case Compare:
		baselineFingerprint, err := fingerprint(input.BaselineDir)
		if err != nil {
			return Decision{}, fmt.Errorf("baseline payload: %w", err)
		}
		changed := candidateFingerprint != baselineFingerprint
		reason := "unchanged"
		if changed {
			reason = "changed"
		}
		return Decision{
			ShouldPublish:       changed,
			Reason:              reason,
			BaselineFingerprint: baselineFingerprint,
		}, nil
	default:
		return Decision{}, fmt.Errorf("invalid release decision mode %d", input.Mode)
	}
}

func fingerprint(directory string) (string, error) {
	if err := verify.Assets(directory, verify.ReleaseAssets()); err != nil {
		return "", err
	}
	digest := sha256.New()
	for _, name := range []string{"geosite.dat", "geoip.dat"} {
		if err := hashFileFrame(digest, directory, name); err != nil {
			return "", err
		}
	}
	installer, err := os.ReadFile(filepath.Join(directory, "install_passwall2_rules.sh"))
	if err != nil {
		return "", fmt.Errorf("read installer: %w", err)
	}
	normalized, err := NormalizeInstaller(installer)
	if err != nil {
		return "", err
	}
	if err := hashFrame(digest, "install_passwall2_rules.sh", int64(len(normalized)), bytes.NewReader(normalized)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func hashFileFrame(digest hash.Hash, directory, name string) error {
	file, err := os.Open(filepath.Join(directory, name))
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}
	return hashFrame(digest, name, info.Size(), file)
}

func hashFrame(digest hash.Hash, name string, size int64, source io.Reader) error {
	if size < 0 {
		return fmt.Errorf("negative content length for %s", name)
	}
	if err := binary.Write(digest, binary.BigEndian, uint64(len(name))); err != nil {
		return fmt.Errorf("frame name length for %s: %w", name, err)
	}
	if _, err := io.WriteString(digest, name); err != nil {
		return fmt.Errorf("frame name %s: %w", name, err)
	}
	if err := binary.Write(digest, binary.BigEndian, uint64(size)); err != nil {
		return fmt.Errorf("frame content length for %s: %w", name, err)
	}
	written, err := io.Copy(digest, source)
	if err != nil {
		return fmt.Errorf("hash content %s: %w", name, err)
	}
	if written != size {
		return fmt.Errorf("content length changed for %s: expected %d, read %d", name, size, written)
	}
	return nil
}
```

执行时若 `defer file.Close()` 被静态检查指出不能报告关闭错误，则改为显式关闭并返回 `close <name>` 错误；不得改变指纹帧格式。`fingerprint` 不导出，publish job 通过同一个 `release-decision --baseline` 命令取得基线指纹。

删除 `internal/app/build.go` 的私有 `releaseAssets`，将 `internal/app/deps.go`、`internal/app/build_test.go` 所有调用替换为 `verify.ReleaseAssets()`。用 `gofmt` 格式化全部 Go 文件。

- [x] **步骤 4：运行测试确认通过**

运行：

```bash
go test ./internal/verify ./internal/passwall ./internal/releasecmp ./internal/app
go test -race ./internal/releasecmp ./internal/verify
go vet ./internal/verify ./internal/passwall ./internal/releasecmp ./internal/app
```

预期：全部 PASS/exit 0；race 不得报告共享资产切片或并发读问题。

- [x] **步骤 5：提交**

```bash
git add internal/verify/assets.go internal/verify/assets_test.go \
  internal/passwall/installer.go internal/passwall/installer_test.go \
  internal/releasecmp/releasecmp.go internal/releasecmp/releasecmp_test.go \
  internal/app/build.go internal/app/deps.go internal/app/build_test.go
git commit -m "feat(T1): 增加规范化发布载荷比较器"
```

### 任务 2：发布判定 CLI 契约

**文件**：
- 创建：`internal/app/release_decision.go`
- 修改：`internal/app/commands.go`
- 修改：`internal/app/commands_test.go`
- 修改：`internal/cli/run.go`
- 修改：`internal/cli/run_test.go`

**接口**：
- 消费：`releasecmp.Input`、`releasecmp.Mode`、`releasecmp.Decide(Input) (Decision, error)`。
- 产出：`app.ReleaseDecisionOptions`、`app.ReleaseDecision(ReleaseDecisionOptions, io.Writer) error`、`cli.Commands.ReleaseDecision`，以及稳定 stdout 三键 `should_publish`/`reason`/`baseline_fingerprint`。

- [ ] **步骤 1：写失败测试**

在 `internal/cli/run_test.go` 增加 `TestReleaseDecisionCommandIsDispatched`：传入只记录调用参数的 stub `ReleaseDecision`，执行 `release-decision --candidate publish --force`，断言 exit 0、参数原样传递；同时把 help 精确断言为包含 `bootstrap|build|verify|release-decision`。

在 `internal/app/commands_test.go` 加入以下表驱动红测：

```go
func Test发布判定模式缺失或冲突(t *testing.T) {
	cases := [][]string{
		{"--candidate", "publish"},
		{"--candidate", "publish", "--baseline", "old", "--first-release"},
		{"--candidate", "publish", "--baseline", "old", "--force"},
		{"--candidate", "publish", "--first-release", "--force"},
	}
	// 每组经 cli.Run 调用 release-decision，断言 exit 2、stdout 为空、stderr 含 exactly one。
}

func TestReleaseDecisionRequiresCandidate(t *testing.T) {
	// 分别给 baseline/first/force，断言均 exit 2 且 stdout 为空。
}

func TestReleaseDecisionPrintsStableOutput(t *testing.T) {
	// 建立有效 candidate/baseline fixture；断言 stdout 恰为：
	// should_publish=false\nreason=unchanged\nbaseline_fingerprint=<64hex>\n
}

func Test首次或强制模式下候选损坏CLI(t *testing.T) {
	// first/force 分别指向坏候选；断言 exit 1、stdout 为空、stderr 含 candidate payload。
}
```

fixture 复用与任务 1 相同的真实六资产写法，但放在 `internal/app` 测试包自己的 helper 中，避免导出测试专用生产 API。另测未知 flag、额外位置参数为 exit 2；可读性/资产错误为 exit 1。

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
go test ./internal/cli ./internal/app -run 'ReleaseDecision|发布判定|首次或强制'
```

预期：FAIL，报 `Commands.ReleaseDecision undefined`、未知命令或新解析函数不存在；保留失败输出。

- [ ] **步骤 3：写最小实现**

扩展 `internal/cli/run.go`：

```go
type Commands struct {
	Bootstrap       Command
	Build           Command
	Verify          Command
	ReleaseDecision Command
}
```

help 改为 `usage: geodata-build <bootstrap|build|verify|release-decision> [options]`，switch 增加 `case "release-decision": command = commands.ReleaseDecision`，保留既有 unknown=2、未接线=1、`UsageError`=2 逻辑。

创建 `internal/app/release_decision.go`：

```go
package app

import (
	"fmt"
	"io"

	"clash-rules-srs/internal/releasecmp"
)

type ReleaseDecisionOptions struct {
	CandidateDir string
	BaselineDir  string
	Mode         releasecmp.Mode
}

func ReleaseDecision(options ReleaseDecisionOptions, out io.Writer) error {
	decision, err := releasecmp.Decide(releasecmp.Input{
		CandidateDir: options.CandidateDir,
		BaselineDir:  options.BaselineDir,
		Mode:         options.Mode,
	})
	if err != nil {
		return err
	}
	baseline := decision.BaselineFingerprint
	if baseline == "" {
		baseline = "none"
	}
	_, err = fmt.Fprintf(out, "should_publish=%t\nreason=%s\nbaseline_fingerprint=%s\n",
		decision.ShouldPublish, decision.Reason, baseline)
	return err
}
```

在 `internal/app/commands.go` 的 `Commands()` 中接线，必须传真实 stdout：

```go
ReleaseDecision: func(_ context.Context, args []string, out, _ io.Writer) error {
	options, err := parseReleaseDecisionOptions(args)
	if err != nil {
		return err
	}
	return ReleaseDecision(options, out)
},
```

解析器使用 `flag.FlagSet`：

```go
func parseReleaseDecisionOptions(args []string) (ReleaseDecisionOptions, error) {
	set := newFlagSet("release-decision")
	var options ReleaseDecisionOptions
	var firstRelease, force bool
	set.StringVar(&options.CandidateDir, "candidate", "", "candidate six-asset directory")
	set.StringVar(&options.BaselineDir, "baseline", "", "baseline six-asset directory")
	set.BoolVar(&firstRelease, "first-release", false, "publish when latest is explicitly absent")
	set.BoolVar(&force, "force", false, "publish a verified candidate without a baseline")
	if err := parseFlags(set, args); err != nil {
		return ReleaseDecisionOptions{}, err
	}
	if options.CandidateDir == "" {
		return ReleaseDecisionOptions{}, usageError(fmt.Errorf("--candidate is required"))
	}
	selected := 0
	if options.BaselineDir != "" { selected++ }
	if firstRelease { selected++ }
	if force { selected++ }
	if selected != 1 {
		return ReleaseDecisionOptions{}, usageError(fmt.Errorf("exactly one of --baseline, --first-release, or --force is required"))
	}
	switch {
	case options.BaselineDir != "":
		options.Mode = releasecmp.Compare
	case firstRelease:
		options.Mode = releasecmp.FirstRelease
	case force:
		options.Mode = releasecmp.Force
	}
	options.CandidateDir = filepath.Clean(options.CandidateDir)
	if options.BaselineDir != "" {
		options.BaselineDir = filepath.Clean(options.BaselineDir)
	}
	return options, nil
}
```

将单行 `if selected++` 展开为 gofmt 接受的标准多行 Go 语句。确保 `Decide` 成功前不写 stdout，避免错误路径留下 workflow 可接受的部分决策。

- [ ] **步骤 4：运行测试确认通过**

运行：

```bash
go test ./internal/cli ./internal/app
go vet ./internal/cli ./internal/app
```

再用临时测试 fixture 或任务 1 测试生成的目录运行三种 CLI，确认输出严格为三行且没有日志混入 stdout。预期全部 PASS/exit 0。

- [ ] **步骤 5：提交**

```bash
git add internal/app/release_decision.go internal/app/commands.go internal/app/commands_test.go \
  internal/cli/run.go internal/cli/run_test.go
git commit -m "feat(T2): 接入发布变化判定命令"
```

### 任务 3：GitHub Actions 增量发布、结构契约与文档

**文件**：
- 修改：`.github/workflows/build.yml`
- 修改：`internal/app/workflow_contract_test.go`
- 修改：`internal/app/docs_contract_test.go`
- 修改：`README.md`
- 修改：`context.md`

**接口**：
- 消费：`geodata-build release-decision --candidate DIR (--baseline DIR|--first-release|--force)` 的三行 stdout。
- 产出：build job outputs `should_publish`、`reason`、`baseline_release_id`、`baseline_tag`、`baseline_fingerprint`；publish job 以这五项绑定同一基线身份。

- [ ] **步骤 1：写失败测试**

先重写 `internal/app/workflow_contract_test.go`，用 `go.yaml.in/yaml/v3` 解析 YAML，而不是只扫描整个文件。定义最小结构：

```go
type workflowDocument struct {
	On struct {
		WorkflowDispatch struct {
			Inputs map[string]struct {
				Type     string `yaml:"type"`
				Default  bool   `yaml:"default"`
				Required bool   `yaml:"required"`
			} `yaml:"inputs"`
		} `yaml:"workflow_dispatch"`
	} `yaml:"on"`
	Permissions map[string]string      `yaml:"permissions"`
	Concurrency workflowConcurrency    `yaml:"concurrency"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

type workflowConcurrency struct {
	Group            string `yaml:"group"`
	CancelInProgress bool   `yaml:"cancel-in-progress"`
}

type workflowJob struct {
	Needs       string                 `yaml:"needs"`
	If          string                 `yaml:"if"`
	Permissions map[string]string      `yaml:"permissions"`
	Outputs     map[string]string      `yaml:"outputs"`
	Steps       []workflowStep         `yaml:"steps"`
}

type workflowStep struct {
	Name string         `yaml:"name"`
	ID   string         `yaml:"id"`
	If   string         `yaml:"if"`
	Uses string         `yaml:"uses"`
	Run  string         `yaml:"run"`
	With map[string]any `yaml:"with"`
}
```

增加按 `ID` 或 `Name` 在指定 job 内查找 step 的 helper；所有顺序断言比较该 job 的 step 下标。新增以下红测，每个测试都在正确 job/step 结构内断言：

```go
func Test相同内容重复运行Workflow(t *testing.T)         // Artifact if 绑定 should_publish=='true'；publish if 同时绑定输出与默认分支
func Test尚无LatestReleaseWorkflow(t *testing.T)        // resolve step 仅 status=404 选择 first-release
func Test内容相同但人工强制发布Workflow(t *testing.T)    // boolean input 默认 false；force 调用 --force 且仍完整构建/校验
func Test非默认分支强制运行(t *testing.T)               // publish job if 必须包含默认分支表达式
func TestLatest查询不是200或404(t *testing.T)           // case 其他状态 exit 1，不能设置 first-release
func TestLatest查询成功但资产下载失败(t *testing.T)      // 200 分支的 gh release download 未被容错，且先下载后 decision
func Test比较后Latest被外部替换(t *testing.T)            // publish 的远端写入前，依次复核 ID、tag、Go 计算的 fingerprint
func Test变化后草稿Release六资产回读契约(t *testing.T)   // draft→上传六项→API/readback/checksum→公开/latest 的严格顺序
func Test干净Runner完成无变化判定契约(t *testing.T)      // checkout/setup-go/bootstrap/test/build/release-decision，不出现 Python/Node/npm
```

测试还必须断言：顶层与 build 均 `contents: read`，publish `contents: write`；concurrency 仍为 `geodata-release` 且 `cancel-in-progress=false`；build 五个 output 全部引用固定 step output；publish job 内新增 checkout/setup-go，保证复核调用仓库同一 Go 实现。

在 `internal/app/docs_contract_test.go` 给 README/context 两份文档增加必需短语：`release-decision`、`每日完整构建`、`有效产物变化`、`无变化`、`force_publish`、`明确 404`、`ID、tag 和载荷指纹`；继续守卫没有 Python/Node/npm/clash2passwall 生产命令。

- [ ] **步骤 2：运行测试确认失败**

运行：

```bash
go test ./internal/app -run 'Workflow|Docs|Latest|Runner|草稿|强制|相同内容'
```

预期：FAIL；当前 workflow 没有 input、job outputs、基线获取/复核与条件 Artifact，文档也未描述增量发布。若 YAML 解析本身失败，先只修测试结构体标签或 fixture loader，仍须保持行为断言为红。

- [ ] **步骤 3：写最小实现**

将 `.github/workflows/build.yml` 的手动触发改为：

```yaml
  workflow_dispatch:
    inputs:
      force_publish:
        description: Publish a fully verified candidate without comparing latest
        required: false
        type: boolean
        default: false
```

build job 增加五个 outputs，值全部来自有固定 `id: release_decision` 的 step：

```yaml
    outputs:
      should_publish: ${{ steps.release_decision.outputs.should_publish }}
      reason: ${{ steps.release_decision.outputs.reason }}
      baseline_release_id: ${{ steps.release_decision.outputs.baseline_release_id }}
      baseline_tag: ${{ steps.release_decision.outputs.baseline_tag }}
      baseline_fingerprint: ${{ steps.release_decision.outputs.baseline_fingerprint }}
```

在完整构建与 checksum 验证之后加入 `id: release_decision`。该 step 必须按以下算法实现，所有成功路径最终把 CLI 三行追加到 `$GITHUB_OUTPUT`，并另外写入 baseline ID/tag：

1. `force_publish=true`：直接执行 `go run ./cmd/geodata-build release-decision --candidate publish --force`，ID/tag 输出 `none`。
2. 非 force：用带 `Authorization: Bearer $GH_TOKEN`、GitHub API version 和 JSON accept header 的 `curl --fail-with-body` 等价请求保存 latest 同一次响应体，同时用 `--write-out '%{http_code}'` 单独取得状态；curl 网络错误必须原样非零退出，不能进入状态分支。
3. HTTP 404：执行 `--candidate publish --first-release`，ID/tag 输出 `none`。
4. HTTP 200：从保存的同一 JSON 响应用 `jq -er '.id'` 与 `jq -er '.tag_name'` 取得 ID/tag；`gh release download "$baseline_tag" --dir "$baseline_dir"` 下载该 tag 的全部资产；执行 `--candidate publish --baseline "$baseline_dir"`。
5. 其他 HTTP 状态：打印 `unexpected latest release status: <status>` 后 exit 1。

不要使用浮动 `releases/latest/download` URL；不要给 `gh release download`、`jq`、checksum 或 CLI 调用加 `|| true`。为避免 `curl --fail-with-body` 把合法 404 当无法分类的进程失败，应采用“传输 exit code”和“HTTP status”分离的写法：先临时关闭 `set -e` 执行 curl，保存 exit code；仅允许 exit code 0 或 HTTP 404 对应的 22 继续进入 case，其他 curl exit code 立即失败。

Artifact step 增加：

```yaml
        if: steps.release_decision.outputs.should_publish == 'true'
```

publish job 条件改为同时要求成功的 build output 和默认分支：

```yaml
    if: needs.build.result == 'success' && needs.build.outputs.should_publish == 'true' && github.ref == format('refs/heads/{0}', github.event.repository.default_branch)
```

publish job 在 download-artifact 前增加与 build 相同固定 pin 的 checkout/setup-go；checkout 继续 `persist-credentials: false`。在现有 `Stage, read back, and publish exact Release` 之前新增“Recheck release baseline before write” step，设置 `GH_TOKEN`，并按 `needs.build.outputs.reason` 严格分支：

- `forced`：运行 `release-decision --candidate publish --force` 仅复核候选，不下载旧基线。
- `first-release`：重新查询 latest；只有明确 404 才通过，200 或传输/其他 HTTP 状态均失败。
- `changed`：重新查询 latest 同一次响应体，要求当前 ID 和 tag 分别精确等于 `needs.build.outputs.baseline_release_id`、`baseline_tag`；按当前 tag 下载全部资产到新临时目录；运行 `release-decision --candidate publish --baseline <dir>`，从输出取 `baseline_fingerprint`，要求精确等于 build output，并要求复核结果仍为 `should_publish=true`、`reason=changed`。
- 任何其他 reason：exit 1。

该 step 必须位于 `gh release create` 所在 step 前；不要复写 Go 指纹算法。现有 draft trap、六项上传、API 资产集合/target/tag SHA 回读、下载 checksum、公开 latest、公开后 tag SHA 复核全部保留。

更新 `README.md` 的“CI 与发布边界”和 `context.md` 的 CLI/发布章节，准确写明：每日仍完整拉取、编译、探针和验证；规范化三主资产无变化时没有 Artifact/tag/Release；首次仅明确 404；不可信 latest 默认失败；默认分支手动 `force_publish=true` 是修复通道但不放松候选和回读；普通发布写入前复核 ID、tag、载荷指纹。补充本地 CLI 三种调用示例和稳定三行输出，不承诺本地模拟等价于真实 GitHub 验收。

- [ ] **步骤 4：运行测试确认通过**

运行：

```bash
go test ./internal/app -run 'Workflow|Docs|Latest|Runner|草稿|强制|相同内容'
go test ./...
go vet ./...
```

再运行以下只读结构检查：

```bash
git diff --check
rg -n 'python|setup-python|node |npm |releases/latest/download' .github/workflows/build.yml README.md context.md
```

预期：Go 测试/vet 全部 exit 0，`git diff --check` 无输出；最后的 `rg` 只允许文档中的“不需要/不得使用”说明，不得命中 workflow 生产步骤或浮动 latest 下载 URL。

- [ ] **步骤 5：提交**

```bash
git add .github/workflows/build.yml internal/app/workflow_contract_test.go \
  internal/app/docs_contract_test.go README.md context.md
git commit -m "feat(T3): 仅在有效产物变化时发布"
```

### 任务 4：验收（acceptance-qa）

> 本任务由 executing-plans 收尾审查阶段触发 acceptance-qa 按下表执行，不参与逐任务连续执行；报告与证据落盘特性目录 `acceptance/` 子目录。没有仓库 Release 写权限或 GitHub-hosted runner 时必须如实记录 `DEFERRED` 与缺少的授权，不得用本地模拟标成通过。

| Scenario / 检查项 | 维度 | 执行方式 | 目标 | 阈值/预期 | 验收证据 |
|---|---|---|---|---|---|
| 变化后草稿 Release 六资产回读 | release | 验收任务 (D) | 授权测试仓库的一次默认分支 workflow_dispatch | draft 精确六资产；target commit、tag SHA、三 checksum 全通过后才公开/latest | workflow run URL、Release API JSON、六资产名与 checksum 日志；无授权为 `DEFERRED` |
| 干净环境完成全链路 | e2e | 验收任务 (D) | `git archive HEAD` 解包到临时目录，环境不额外安装 Python/Node/npm/全局 geoview/geoip | bootstrap、`go test ./...`、integration、完整 build、严格六资产验证全部 exit 0 | 环境版本、完整命令日志、`find publish -maxdepth 1 -type f` 六项清单 |
| 干净 runner 完成无变化判定 | e2e | 验收任务 (D) | GitHub-hosted Ubuntu 对可信 latest 连续运行两次 | 第二次 build 成功且 `should_publish=false/reason=unchanged`，无 Artifact，publish skipped，远端 tag/Release/latest 不变 | 两次 run URL、job outputs、Artifact 列表、前后 latest ID/tag；无 runner/授权为 `DEFERRED` |

验收前先运行 `git status --short`，确保使用的是任务 1–3 的已提交 worktree。真实远端测试只能在明确授权的仓库范围内执行，不得擅自向其他仓库发布。

### 任务 5：合并与清理

- [ ] **步骤 1：全量验证**

在 worktree 内运行：

```bash
go test ./...
go vet ./...
go test -tags=integration ./internal/app
go test -race ./internal/releasecmp ./internal/verify ./internal/cli
git diff --check
git status --short
```

预期：测试/vet/race/diff check 全绿；status 只允许 acceptance 报告或已知待提交的 spec 状态记录。代码/报告有未提交内容则先提交明确范围，失败则修复后才进入合并。

- [ ] **步骤 2：合并回来源分支**

先在主工作区运行 `git status --short`。若存在任何未提交改动，必须停下让用户选择先提交、暂存到安全位置或延期合并，不得自动 stash、代为提交、丢弃或覆盖。

主工作区干净后运行：

```bash
cd "$(dirname "$(git rev-parse --git-common-dir)")"
git merge plan/2026-08-02-incremental-geodata-release
```

合并冲突或出现新的未提交改动则停下向计划作者确认，不强行合并。

- [ ] **步骤 3：清理**

```bash
git worktree remove .worktrees/plan-2026-08-02-incremental-geodata-release
git branch -d plan/2026-08-02-incremental-geodata-release
```

- [ ] **步骤 4：sync_commit 锚定**

```bash
SYNC=$(git rev-parse HEAD)
# 将下列 spec frontmatter 的 sync_commit: null 更新为此完整 $SYNC：
# .spec-dev/2026-08-02-incremental-geodata-release/spec/incremental-geodata-release-design.md
git add .spec-dev/2026-08-02-incremental-geodata-release/spec/incremental-geodata-release-design.md
git commit -m "chore(spec): sync_commit 锚定 ${SYNC:0:7}"
```

此后 `git diff <sync_commit>..HEAD -- cmd/geodata-build internal/app internal/releasecmp .github/workflows/build.yml README.md context.md` 即该 spec 上次确认同步以来的代码变化。

任务 0 未由本计划建立 worktree时，只执行步骤 1 与步骤 4；步骤 2–3 交回原有隔离机制收尾并记录原因。

## Spec 到测试与任务追踪

| Requirement / Scenario | 红测或验收 | 实现任务 |
|---|---|---|
| 仅在有效产物变化时发布 / 相同内容重复运行 | `Test相同内容重复运行`、`Test相同内容重复运行Workflow` | T1、T3 |
| 仅在有效产物变化时发布 / 任一 dat 发生变化 | `Test任一Dat发生变化` | T1 |
| 仅在有效产物变化时发布 / 安装器逻辑发生变化 | `Test安装器逻辑发生变化` | T1 |
| 仅在有效产物变化时发布 / 尚无 latest Release | `Test尚无LatestRelease`、`Test尚无LatestReleaseWorkflow` | T1、T3 |
| 安装器只忽略发布绑定字段 / 安装器仅 Release tag 不同 | `Test安装器仅ReleaseTag不同` | T1 |
| 安装器只忽略发布绑定字段 / 安装器发布绑定字段异常 | `Test安装器发布绑定字段异常` | T1 |
| 手动运行可强制发布已验证候选 / 内容相同但人工强制发布 | `Test内容相同但人工强制发布`、`Test内容相同但人工强制发布Workflow` | T1、T3 |
| 手动运行可强制发布已验证候选 / 非默认分支强制运行 | `Test非默认分支强制运行` | T3 |
| 基线错误严格失败 / latest 六资产损坏 | `TestLatest六资产损坏` | T1 |
| 基线错误严格失败 / latest 查询不是 200 或 404 | `TestLatest查询不是200或404` | T3 |
| 基线错误严格失败 / latest 查询成功但资产下载失败 | `TestLatest查询成功但资产下载失败` | T3 |
| 发布判定 CLI 要求恰好一种模式 / 模式缺失或冲突 | `Test发布判定模式缺失或冲突` | T2 |
| 发布判定 CLI 要求恰好一种模式 / first/force 候选损坏 | `Test首次或强制模式下候选损坏`、`Test首次或强制模式下候选损坏CLI` | T1、T2 |
| 普通发布复核完整基线身份 / 比较后 latest 被外部替换 | `Test比较后Latest被外部替换` | T3 |
| Release 资产保持严格六项 / 变化后草稿 Release 六资产回读 | `Test变化后草稿Release六资产回读契约` + T4 远端验收 | T3、T4 |
| 第一方构建链保持全 Go / 干净环境完成全链路 | T4 clean archive e2e；既有 integration 测试纳入全量门禁 | T3、T4 |
| 第一方构建链保持全 Go / 干净 runner 完成无变化判定 | `Test干净Runner完成无变化判定契约` + T4 GitHub-hosted 验收 | T3、T4 |
