package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
			"gh-proxy.com → ghfast.top → GitHub 官方",
			"每个源 60 秒",
			"shadowrocket.conf",
			"shadowrocket.conf.sha256sum",
			"https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf",
			"二维码",
			"shadowrocket://config/add/",
			"force_publish=true",
			"八资产",
		} {
			if !bytes.Contains(data, []byte(required)) {
				t.Errorf("%s missing %q", name, required)
			}
		}
		for _, forbidden := range []string{
			"python scripts/",
			"npm --prefix",
			"clash2passwall.js",
			"精确六个文件",
			"候选六资产",
			"规范化三主资产",
			"六资产验证",
			"精确比较六个文件",
		} {
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
