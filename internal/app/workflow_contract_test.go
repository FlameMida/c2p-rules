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
	for _, required := range []string{
		"needs.build.result == 'success'",
		"needs.build.outputs.should_publish == 'true'",
		"github.event.repository.default_branch",
	} {
		if !strings.Contains(publish.If, required) {
			t.Fatalf("publish if=%q missing %q", publish.If, required)
		}
	}
}

func Test尚无LatestReleaseWorkflow(t *testing.T) {
	decision := stepByID(t, loadWorkflow(t).Jobs["build"], "release_decision")
	assertRunContains(t, decision, "404)", "--first-release", "baseline_release_id=none", "baseline_tag=none")
	if strings.Index(decision.Run, "404)") > strings.Index(decision.Run, "--first-release") {
		t.Fatalf("first-release is not confined to the 404 branch:\n%s", decision.Run)
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
	assertRunContains(t, decision, "--force", "FORCE_PUBLISH")
	for _, name := range []string{"Build exact release payload", "Verify all checksums"} {
		if stepIndexByName(t, build, name) >= decisionIndex {
			t.Fatalf("%s does not precede force decision", name)
		}
	}
}

func Test非默认分支强制运行(t *testing.T) {
	condition := loadWorkflow(t).Jobs["publish"].If
	if !strings.Contains(condition, "github.ref == format('refs/heads/{0}', github.event.repository.default_branch)") {
		t.Fatalf("publish condition lacks default branch gate: %q", condition)
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
	assertRunContains(t, decision, `gh release download "$baseline_tag"`, `--baseline "$baseline_dir"`)
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
	assertRunContains(t, recheck,
		"current_release_id", "BASELINE_RELEASE_ID",
		"current_tag", "BASELINE_TAG",
		"baseline_fingerprint", "BASELINE_FINGERPRINT",
		"go run ./cmd/geodata-build release-decision",
		"should_publish=true", "reason=changed",
	)
}

func Test变化后草稿Release六资产回读契约(t *testing.T) {
	publish := loadWorkflow(t).Jobs["publish"]
	stage := stepByName(t, publish, "Stage, read back, and publish exact Release")
	previous := -1
	for _, fragment := range []string{
		"gh release create", "--draft", "gh release upload",
		`.assets[].name`, `target_commitish`, `git/ref/tags/$TAG`,
		`gh release download "$TAG"`, "sha256sum -c geoip.dat.sha256sum",
		`gh release edit "$TAG" --draft=false --latest`,
	} {
		index := strings.Index(stage.Run, fragment)
		if index < 0 || index < previous {
			t.Fatalf("release transaction missing/out of order %q:\n%s", fragment, stage.Run)
		}
		previous = index
	}
	for _, name := range []string{
		"geoip.dat", "geoip.dat.sha256sum", "geosite.dat", "geosite.dat.sha256sum",
		"install_passwall2_rules.sh", "install_passwall2_rules.sh.sha256sum",
	} {
		if strings.Count(stage.Run, name) < 2 {
			t.Fatalf("release transaction does not upload/read back %s", name)
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
