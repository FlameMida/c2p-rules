package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDocsDescribeGoOnlyWorkflowAndManagedInstall(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range []string{"README.md", "context.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{
			"geodata-build bootstrap",
			"geodata-build build",
			"geodata-build verify",
			"custom/geosite",
			"custom/geoip",
			"config/passwall2-groups.yaml",
			"install_passwall2_rules.sh.sha256sum",
			"managed_by=clash-rules-srs",
			"回滚",
			"release-decision",
			"每日完整构建",
			"有效产物变化",
			"无变化",
			"force_publish",
			"明确 404",
			"ID、tag 和载荷指纹",
		} {
			if !bytes.Contains(data, []byte(required)) {
				t.Errorf("%s missing %q", name, required)
			}
		}
		for _, forbidden := range []string{"python scripts/", "npm --prefix", "clash2passwall.js"} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Errorf("%s contains legacy instruction %q", name, forbidden)
			}
		}
		previous := -1
		for _, step := range []string{
			"事务顺序固定为",
			"备份",
			"staging UCI",
			"安装并提交 live 配置",
			"调用 updater 更新两个 dat",
			"校验两个 dat SHA-256",
			"持久 URL 切到 `latest`",
		} {
			index := bytes.Index(data, []byte(step))
			if index < 0 || index <= previous {
				t.Errorf("%s does not document install step %q in order", name, step)
				break
			}
			previous = index
		}
	}
}
