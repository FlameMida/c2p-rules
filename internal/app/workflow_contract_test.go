package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

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
	Needs       string            `yaml:"needs"`
	If          string            `yaml:"if"`
	Permissions map[string]string `yaml:"permissions"`
	Outputs     map[string]string `yaml:"outputs"`
	Steps       []workflowStep    `yaml:"steps"`
}

type workflowStep struct {
	Name string            `yaml:"name"`
	ID   string            `yaml:"id"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]any    `yaml:"with"`
}

func Test相同内容重复运行Workflow(t *testing.T) {
	document := loadWorkflow(t)
	build := document.Jobs["build"]
	upload := stepByUses(t, build, "actions/upload-artifact@")
	if !strings.Contains(upload.If, "steps.release_decision.outputs.should_publish == 'true'") {
		t.Fatalf("artifact if=%q", upload.If)
	}
	publish := document.Jobs["publish"]
	want := "needs.build.result == 'success' && needs.build.outputs.should_publish == 'true' && github.ref == format('refs/heads/{0}', github.event.repository.default_branch)"
	if strings.TrimSpace(publish.If) != want {
		t.Fatalf("publish if=%q want=%q", publish.If, want)
	}
}

func Test尚无LatestReleaseWorkflow(t *testing.T) {
	decision := stepByID(t, loadWorkflow(t).Jobs["build"], "release_decision")
	if !containsShellBlock(decision.Run, `case "$latest_status" in`) {
		t.Fatalf("release decision does not branch on latest_status:\n%s", decision.Run)
	}
	branch := shellCaseBranch(t, decision.Run, "404)")
	assertTextContains(t, branch, "--first-release", `--candidate-tag "$RELEASE_TAG"`)
	if strings.Count(decision.Run, "--first-release") != 1 {
		t.Fatalf("first-release must appear only in the 404 branch:\n%s", decision.Run)
	}
	assertRunContains(t, decision, "baseline_release_id=none", "baseline_tag=none")

	recheck := stepByName(t, loadWorkflow(t).Jobs["publish"], "Recheck release baseline before write")
	firstReleaseBranch := shellCaseBranch(t, recheck.Run, "first-release)")
	firstReleaseBlock := `if [ "$latest_status" != "404" ]; then
  echo "latest appeared after first-release decision" >&2
  exit 1
fi`
	if !containsShellBlock(firstReleaseBranch, firstReleaseBlock) {
		t.Fatalf("publish does not require latest to remain 404:\n%s", recheck.Run)
	}
}

func Test内容相同但人工强制发布Workflow(t *testing.T) {
	document := loadWorkflow(t)
	input, exists := document.On.WorkflowDispatch.Inputs["force_publish"]
	if !exists || input.Type != "boolean" || input.Default || input.Required {
		t.Fatalf("force_publish=%+v exists=%t", input, exists)
	}
	build := document.Jobs["build"]
	decisionIndex := stepIndexByID(t, build, "release_decision")
	decision := build.Steps[decisionIndex]
	assertRunContains(t, decision, "--force", "FORCE_PUBLISH", `--candidate-tag "$RELEASE_TAG"`)
	for _, name := range []string{"Build exact release payload", "Verify all checksums"} {
		if stepIndexByName(t, build, name) >= decisionIndex {
			t.Fatalf("%s does not precede force decision", name)
		}
	}
}

func Test非默认分支强制运行(t *testing.T) {
	condition := loadWorkflow(t).Jobs["publish"].If
	want := "needs.build.result == 'success' && needs.build.outputs.should_publish == 'true' && github.ref == format('refs/heads/{0}', github.event.repository.default_branch)"
	if strings.TrimSpace(condition) != want {
		t.Fatalf("publish condition=%q want=%q", condition, want)
	}
}

func TestLatest查询不是200或404(t *testing.T) {
	decision := stepByID(t, loadWorkflow(t).Jobs["build"], "release_decision")
	assertRunContains(t, decision, "200)", "404)", "unexpected latest release status", "exit 1")
	if strings.Contains(decision.Run, "gh release view") {
		t.Fatalf("ambiguous gh release view failure is used:\n%s", decision.Run)
	}
}

func TestLatest查询成功但资产下载失败(t *testing.T) {
	decision := stepByID(t, loadWorkflow(t).Jobs["build"], "release_decision")
	assertRunContains(t, decision,
		`gh release download "$baseline_tag"`,
		`--baseline "$baseline_dir"`,
		`--baseline-tag "$baseline_tag"`,
		`--candidate-tag "$RELEASE_TAG"`,
	)
	download := strings.Index(decision.Run, `gh release download "$baseline_tag"`)
	compare := strings.Index(decision.Run, `--baseline "$baseline_dir"`)
	if download < 0 || compare <= download {
		t.Fatalf("baseline download/compare order is wrong:\n%s", decision.Run)
	}
	for _, line := range strings.Split(decision.Run, "\n") {
		if strings.Contains(line, "gh release download") && strings.Contains(line, "|| true") {
			t.Fatalf("download failure is ignored: %q", line)
		}
	}
}

func Test比较后Latest被外部替换(t *testing.T) {
	publish := loadWorkflow(t).Jobs["publish"]
	recheckIndex := stepIndexByName(t, publish, "Recheck release baseline before write")
	stageIndex := stepIndexByName(t, publish, "Stage, read back, and publish exact Release")
	if recheckIndex >= stageIndex {
		t.Fatalf("recheck index=%d stage index=%d", recheckIndex, stageIndex)
	}
	recheck := publish.Steps[recheckIndex]
	changedBranch := shellCaseBranch(t, recheck.Run, "changed)")
	for _, check := range []struct {
		source string
		block  string
	}{
		{`current_release_id=$(jq -er '.id' "$latest_response")`, "if [ \"$current_release_id\" != \"$BASELINE_RELEASE_ID\" ]; then\n  echo \"baseline release ID changed\" >&2\n  exit 1\nfi"},
		{`current_tag=$(jq -er '.tag_name' "$latest_response")`, "if [ \"$current_tag\" != \"$BASELINE_TAG\" ]; then\n  echo \"baseline tag changed\" >&2\n  exit 1\nfi"},
		{`baseline_fingerprint=$(sed -n 's/^baseline_fingerprint=//p' "$recheck_decision")`, "if [ \"$baseline_fingerprint\" != \"$BASELINE_FINGERPRINT\" ]; then\n  echo \"baseline fingerprint changed\" >&2\n  exit 1\nfi"},
	} {
		sourceIndex := shellBlockIndex(changedBranch, check.source)
		blockIndex := shellBlockIndex(changedBranch, check.block)
		if sourceIndex < 0 || blockIndex <= sourceIndex {
			t.Fatalf("recheck source/block order invalid for %q:\n%s", check.source, changedBranch)
		}
	}
	assertRunContains(t, recheck,
		"go run ./cmd/geodata-build release-decision",
		`--candidate-tag "$candidate_tag"`,
		`--baseline-tag "$current_tag"`,
		"should_publish=true", "reason=changed",
	)
}

func Test变化后草稿Release六资产回读契约(t *testing.T) {
	publish := loadWorkflow(t).Jobs["publish"]
	stage := stepByName(t, publish, "Stage, read back, and publish exact Release")
	strictReadback := "go run ./cmd/geodata-build release-decision \\\n  --candidate \"$readback\" \\\n  --candidate-tag \"$TAG\" \\\n  --force"
	previous := -1
	for _, fragment := range []string{
		"gh release create", "--draft",
		`release_id=$(gh release view "$TAG" --json databaseId --jq '.databaseId')`,
		"gh release upload", `releases/$release_id`,
		`.assets[].name`, `.tag_name`, `.target_commitish`,
		`gh release download "$TAG"`, "sha256sum -c geoip.dat.sha256sum",
		strictReadback,
		`gh release edit "$TAG" --draft=false --latest`,
		`git/ref/tags/$TAG`,
	} {
		index := strings.Index(stage.Run, fragment)
		if index < 0 || index < previous {
			t.Fatalf("release transaction missing/out of order %q:\n%s", fragment, stage.Run)
		}
		previous = index
	}
	upload := "gh release upload \"$TAG\" \\\n  publish/geoip.dat \\\n  publish/geoip.dat.sha256sum \\\n  publish/geosite.dat \\\n  publish/geosite.dat.sha256sum \\\n  publish/install_passwall2_rules.sh \\\n  publish/install_passwall2_rules.sh.sha256sum"
	if !strings.Contains(stage.Run, upload) {
		t.Fatalf("release upload is not exact six assets:\n%s", stage.Run)
	}
	for _, block := range []string{
		`test "$(jq -r '.draft' "$draft_response")" = "true"`,
		`test "$(jq -r '.tag_name' "$draft_response")" = "$TAG"`,
		"target=$(jq -er '.target_commitish' \"$draft_response\")\ntest \"$target\" = \"$GITHUB_SHA\"",
		"tag_sha=$(gh api \"repos/$GITHUB_REPOSITORY/git/ref/tags/$TAG\" --jq '.object.sha')\ntest \"$tag_sha\" = \"$GITHUB_SHA\"",
	} {
		if !strings.Contains(stage.Run, block) {
			t.Fatalf("release transaction misses strict equality %q:\n%s", block, stage.Run)
		}
	}
	if strings.Contains(stage.Run, "releases/tags/$TAG") {
		t.Fatalf("draft release is queried through the published-tag endpoint:\n%s", stage.Run)
	}
	if strings.Count(stage.Run, "git/ref/tags/$TAG") != 1 {
		t.Fatalf("Git tag must be checked exactly once after publication:\n%s", stage.Run)
	}
}

func TestShanghaiTimestampReleaseNamingContract(t *testing.T) {
	document := loadWorkflow(t)
	build := document.Jobs["build"]
	if got, want := build.Outputs["release_tag"], "${{ steps.release_context.outputs.release_tag }}"; got != want {
		t.Fatalf("release_tag output=%q want=%q", got, want)
	}
	context := stepByID(t, build, "release_context")
	assertRunContains(t, context,
		`TAG=$(TZ=Asia/Shanghai date +'%Y%m%d%H%M%S')`,
		`echo "RELEASE_TAG=$TAG" >> "$GITHUB_ENV"`,
		`echo "release_tag=$TAG" >> "$GITHUB_OUTPUT"`,
	)

	publish := document.Jobs["publish"]
	for _, name := range []string{"Recheck release baseline before write", "Stage, read back, and publish exact Release"} {
		step := stepByName(t, publish, name)
		if got, want := step.Env["RELEASE_TAG"], "${{ needs.build.outputs.release_tag }}"; got != want {
			t.Fatalf("%s RELEASE_TAG=%q want=%q", name, got, want)
		}
	}
	recheck := stepByName(t, publish, "Recheck release baseline before write")
	assertRunContains(t, recheck, `candidate_tag="$RELEASE_TAG"`)
	stage := stepByName(t, publish, "Stage, read back, and publish exact Release")
	assertRunContains(t, stage, `TAG="$RELEASE_TAG"`, `--title "r${TAG}"`)

	for _, job := range document.Jobs {
		for _, step := range job.Steps {
			if strings.Contains(step.Run, "GITHUB_RUN_ID") || strings.Contains(step.Run, "GITHUB_RUN_ATTEMPT") {
				t.Fatalf("run identity is still used for Release naming in step %q:\n%s", step.Name, step.Run)
			}
		}
	}
}

func Test干净Runner完成无变化判定契约(t *testing.T) {
	build := loadWorkflow(t).Jobs["build"]
	ordered := []string{
		"actions/checkout@", "actions/setup-go@", "Run Go tests", "Bootstrap pinned tools",
		"Run real-tool integration test", "Build exact release payload", "Verify all checksums", "release_decision",
	}
	previous := -1
	for _, marker := range ordered {
		index := stepIndexByMarker(t, build, marker)
		if index <= previous {
			t.Fatalf("step %q index=%d after=%d", marker, index, previous)
		}
		previous = index
	}
	allRuns := ""
	for _, step := range build.Steps {
		allRuns += step.Run + "\n"
	}
	assertTextContains(t, allRuns,
		"go test ./...", "go test -tags=integration ./internal/app",
		"go run ./cmd/geodata-build bootstrap", "go run ./cmd/geodata-build build",
		"go run ./cmd/geodata-build release-decision",
	)
	for _, forbidden := range []string{"setup-python", "python ", "npm ", "node "} {
		if strings.Contains(allRuns, forbidden) {
			t.Fatalf("build contains legacy runtime %q", forbidden)
		}
	}
}

func TestWorkflowPermissionsOutputsAndConcurrency(t *testing.T) {
	document := loadWorkflow(t)
	if document.Permissions["contents"] != "read" || document.Jobs["build"].Permissions["contents"] != "read" {
		t.Fatalf("read permissions top=%v build=%v", document.Permissions, document.Jobs["build"].Permissions)
	}
	if document.Jobs["publish"].Permissions["contents"] != "write" {
		t.Fatalf("publish permissions=%v", document.Jobs["publish"].Permissions)
	}
	if document.Concurrency.Group != "geodata-release" || document.Concurrency.CancelInProgress {
		t.Fatalf("concurrency=%+v", document.Concurrency)
	}
	outputs := document.Jobs["build"].Outputs
	for _, name := range []string{"should_publish", "reason", "baseline_release_id", "baseline_tag", "baseline_fingerprint"} {
		if !strings.Contains(outputs[name], "steps.release_decision.outputs."+name) {
			t.Fatalf("output %s=%q", name, outputs[name])
		}
	}
	publish := document.Jobs["publish"]
	if stepIndexByMarker(t, publish, "actions/checkout@") > stepIndexByMarker(t, publish, "actions/setup-go@") {
		t.Fatal("publish setup-go precedes checkout")
	}
}

func loadWorkflow(t *testing.T) workflowDocument {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "build.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var document workflowDocument
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse workflow YAML: %v", err)
	}
	return document
}

func stepByID(t *testing.T, job workflowJob, id string) workflowStep {
	t.Helper()
	return job.Steps[stepIndexByID(t, job, id)]
}

func stepByName(t *testing.T, job workflowJob, name string) workflowStep {
	t.Helper()
	return job.Steps[stepIndexByName(t, job, name)]
}

func stepByUses(t *testing.T, job workflowJob, prefix string) workflowStep {
	t.Helper()
	return job.Steps[stepIndexByMarker(t, job, prefix)]
}

func stepIndexByID(t *testing.T, job workflowJob, id string) int {
	t.Helper()
	for index, step := range job.Steps {
		if step.ID == id {
			return index
		}
	}
	t.Fatalf("step id %q not found", id)
	return -1
}

func stepIndexByName(t *testing.T, job workflowJob, name string) int {
	t.Helper()
	for index, step := range job.Steps {
		if step.Name == name {
			return index
		}
	}
	t.Fatalf("step name %q not found", name)
	return -1
}

func stepIndexByMarker(t *testing.T, job workflowJob, marker string) int {
	t.Helper()
	var available []string
	for index, step := range job.Steps {
		available = append(available, step.Name+step.ID+step.Uses)
		if strings.Contains(step.ID, marker) || strings.Contains(step.Name, marker) || strings.HasPrefix(step.Uses, marker) {
			return index
		}
	}
	t.Fatalf("step marker %q not found in %v", marker, available)
	return -1
}

func assertRunContains(t *testing.T, step workflowStep, values ...string) {
	t.Helper()
	assertTextContains(t, step.Run, values...)
}

func assertTextContains(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(text, value) {
			t.Fatalf("missing %q in:\n%s", value, text)
		}
	}
}

func shellCaseBranch(t *testing.T, script, label string) string {
	t.Helper()
	start := strings.Index(script, label)
	if start < 0 {
		t.Fatalf("case branch %q not found:\n%s", label, script)
	}
	rest := script[start+len(label):]
	end := strings.Index(rest, ";;")
	if end < 0 {
		t.Fatalf("case branch %q has no terminator:\n%s", label, script)
	}
	return rest[:end]
}

func containsShellBlock(script, block string) bool {
	return shellBlockIndex(script, block) >= 0
}

func shellBlockIndex(script, block string) int {
	return strings.Index(strings.Join(strings.Fields(script), " "), strings.Join(strings.Fields(block), " "))
}
