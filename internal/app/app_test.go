package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rustyllh/netkit/internal/config"
	"github.com/rustyllh/netkit/internal/linux"
	"github.com/rustyllh/netkit/internal/snapshot"
)

func TestWaitForCheck(t *testing.T) {
	t.Run("服务最终就绪", func(t *testing.T) {
		attempts := 0
		check := waitForCheck(context.Background(), time.Millisecond, func(context.Context) Check {
			attempts++
			if attempts < 3 {
				return failed("health", "not ready")
			}
			return Check{Name: "health", OK: true}
		})
		if !check.OK {
			t.Fatalf("健康检查 = %#v，期望成功", check)
		}
		if attempts != 3 {
			t.Fatalf("检查次数 = %d，期望 3", attempts)
		}
	})

	t.Run("等待超时保留失败详情", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		check := waitForCheck(ctx, time.Millisecond, func(context.Context) Check {
			return failed("health", "connection refused")
		})
		if check.OK {
			t.Fatalf("健康检查 = %#v，期望失败", check)
		}
		if !strings.Contains(check.Detail, "connection refused") || !strings.Contains(check.Detail, "等待服务就绪超时") {
			t.Fatalf("失败详情 = %q，未保留就绪等待信息", check.Detail)
		}
	})
}

type fakeRunner struct{ results map[string]linux.Result }

func (f fakeRunner) Run(_ context.Context, program string, args ...string) (linux.Result, error) {
	key := program
	for _, arg := range args {
		key += " " + arg
	}
	return f.results[key], nil
}

func TestStatus(t *testing.T) {
	cfg, err := config.Load("/tmp/netkit", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	application := New(cfg, fakeRunner{results: map[string]linux.Result{
		"systemctl is-active mihomo.service":    {Stdout: "active"},
		"systemctl is-enabled mihomo.service":   {Stdout: "enabled"},
		"systemctl is-active easytier.service":  {Stdout: "inactive", ExitCode: 3},
		"systemctl is-enabled easytier.service": {Stdout: "disabled", ExitCode: 1},
	}})
	result := application.Status(context.Background())
	if result.OK {
		t.Fatal("Status().OK = true, want false")
	}
	if len(result.Checks) != 6 {
		t.Fatalf("checks = %d, want 6", len(result.Checks))
	}
	if !result.Checks[0].OK || !result.Checks[1].OK || result.Checks[2].OK || result.Checks[3].OK {
		t.Fatalf("unexpected checks: %#v", result.Checks)
	}
}

func TestPingEasyTierPeer(t *testing.T) {
	cfg, err := config.Load("/tmp/netkit", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	application := New(cfg, fakeRunner{results: map[string]linux.Result{
		"ping -c 3 -W 2 10.126.126.3": {Stdout: "3 packets transmitted, 3 received"},
	}})
	for _, test := range []struct {
		name string
		ip   string
		ok   bool
	}{
		{"有效地址", "10.126.126.3", true},
		{"无效地址", "not-an-ip", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := application.PingEasyTierPeer(context.Background(), test.ip).OK; got != test.ok {
				t.Fatalf("PingEasyTierPeer().OK = %v, want %v", got, test.ok)
			}
		})
	}
}

func TestMihomoApply(t *testing.T) {
	root := testAppRoot(t)
	cfg, err := config.Load(root, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{results: map[string]linux.Result{
		"/usr/local/bin/mihomo -t -d /etc/mihomo": {Stdout: "configuration is valid"},
		"systemctl daemon-reload":                 {},
		"systemctl restart mihomo.service":        {},
		"systemctl is-active mihomo.service":      {Stdout: "active"},
	}}
	failedHealth := func(context.Context, config.Config) Check { return failed("mihomo proxy health", "unreachable") }
	application := NewWithHealth(cfg, runner, failedHealth)
	result := application.MihomoApply(context.Background(), false)
	if result.OK {
		t.Fatal("健康检查失败的 apply 应失败")
	}
	entries, err := snapshot.New(cfg).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Target != "mihomo" {
		t.Fatalf("快照 = %#v", entries)
	}
}

func TestMihomoRollbackReportsRestartFailureAfterRestore(t *testing.T) {
	root := testAppRoot(t)
	cfg, err := config.Load(root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	store := snapshot.New(cfg)
	manifest, err := store.Create(context.Background(), "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "mihomo", "config", "config.yaml")
	if err := os.WriteFile(configPath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load(root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	application := NewWithHealth(cfg, fakeRunner{results: map[string]linux.Result{
		"systemctl daemon-reload":          {},
		"systemctl restart mihomo.service": {ExitCode: 1, Stderr: "restart failed"},
	}}, func(context.Context, config.Config) Check { return Check{OK: true} })
	result := application.MihomoRollback(context.Background(), manifest.ID, false)
	if result.OK {
		t.Fatal("重启失败的 rollback 应失败")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mixed-port: 7890\n" {
		t.Fatalf("应已恢复配置，得到 %q", data)
	}
}

func TestEasyTierValidationAndApply(t *testing.T) {
	root := testAppRoot(t)
	cfg, err := config.Load(root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{results: map[string]linux.Result{
		"systemctl daemon-reload": {}, "systemctl restart easytier.service": {}, "systemctl is-active easytier.service": {Stdout: "active"},
		"/usr/local/bin/easytier-cli peer": {Stdout: "peer-1"}, "ip -o -4 addr show": {Stdout: "2: easytier inet 10.126.126.4/24"},
	}}
	application := New(cfg, runner)
	if got := application.EasyTierApply(context.Background(), false); !got.OK {
		t.Fatalf("EasyTier apply 失败: %#v", got)
	}
	invalid := filepath.Join(root, "easytier", "config", "config.toml")
	if err := os.WriteFile(invalid, []byte("broken = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := application.ValidateEasyTier(context.Background()); got.OK {
		t.Fatal("无效 TOML 校验应失败")
	}
}

func TestEasyTierRollbackReportsRestartFailureAfterRestore(t *testing.T) {
	root := testAppRoot(t)
	cfg, err := config.Load(root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	store := snapshot.New(cfg)
	manifest, err := store.Create(context.Background(), "easytier")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "easytier", "config", "config.toml")
	if err := os.WriteFile(configPath, []byte("instance_name = 'changed'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load(root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	application := New(cfg, fakeRunner{results: map[string]linux.Result{"systemctl daemon-reload": {}, "systemctl restart easytier.service": {ExitCode: 1, Stderr: "restart failed"}}})
	result := application.EasyTierRollback(context.Background(), manifest.ID, false)
	if result.OK {
		t.Fatal("重启失败的 EasyTier rollback 应失败")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "instance_name = 'test'\n" {
		t.Fatalf("应已恢复配置，得到 %q", data)
	}
}

func testAppRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for relative, content := range map[string]string{"mihomo/config/config.yaml": "mixed-port: 7890\n", "services/mihomo.service": "[Service]\n", "easytier/config/config.toml": "instance_name = 'test'\n", "services/easytier.service": "[Service]\n"} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLogs(t *testing.T) {
	cfg, err := config.Load("/tmp/netkit", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	application := New(cfg, fakeRunner{results: map[string]linux.Result{
		"journalctl --unit mihomo.service --no-pager --output short-iso --since 1h0m0s ago": {Stdout: "service started"},
	}})
	result := application.Logs(context.Background(), "mihomo", time.Hour, false)
	if !result.OK || result.Checks[0].Detail != "service started" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
