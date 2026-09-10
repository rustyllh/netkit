// Package app 包含与命令行无关的 Netkit 用例编排。
package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/rustyllh/netkit/internal/audit"
	"github.com/rustyllh/netkit/internal/config"
	"github.com/rustyllh/netkit/internal/linux"
	"github.com/rustyllh/netkit/internal/snapshot"
	"github.com/rustyllh/netkit/internal/systemd"
)

// Check 表示一项可独立评估的诊断结果。
type Check struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

// CreateBackup 创建指定网络配置的可验证快照并写入审计事件。
func (a Application) CreateBackup(ctx context.Context, target string) Result {
	manifest, err := snapshot.New(a.config.RootDir).Create(ctx, target)
	if err != nil {
		return result("backup create", []Check{failed("backup", err.Error())})
	}
	if err := audit.Append(ctx, a.config.RootDir, audit.Event{Operation: "backup create", Target: target, Snapshot: manifest.ID, Result: "success"}); err != nil {
		return result("backup create", []Check{failed("audit", err.Error())})
	}
	return result("backup create", []Check{{Name: "backup", OK: true, Severity: "error", Detail: manifest.ID}})
}

// ListBackups 列出已有快照。
func (a Application) ListBackups(ctx context.Context) Result {
	manifests, err := snapshot.New(a.config.RootDir).List(ctx)
	if err != nil {
		return result("backup list", []Check{failed("backup", err.Error())})
	}
	checks := make([]Check, 0, len(manifests))
	for _, manifest := range manifests {
		checks = append(checks, Check{Name: "backup", OK: true, Severity: "error", Detail: manifest.ID + " " + manifest.Target})
	}
	return result("backup list", checks)
}

// VerifyBackup 验证快照完整性并记录审计事件。
func (a Application) VerifyBackup(ctx context.Context, id string) Result {
	manifest, err := snapshot.New(a.config.RootDir).Verify(ctx, id)
	if err != nil {
		return result("backup verify", []Check{failed("backup", err.Error())})
	}
	if err := audit.Append(ctx, a.config.RootDir, audit.Event{Operation: "backup verify", Target: manifest.Target, Snapshot: id, Result: "success"}); err != nil {
		return result("backup verify", []Check{failed("audit", err.Error())})
	}
	return result("backup verify", []Check{{Name: "backup", OK: true, Severity: "error", Detail: id}})
}

// Result 表示可供机器读取的操作结果。
type Result struct {
	OK        bool     `json:"ok"`
	Operation string   `json:"operation"`
	Checks    []Check  `json:"checks"`
	Warnings  []string `json:"warnings,omitempty"`
}

// Application 协调 Netkit 的各项用例。
type Application struct {
	config  config.Config
	runner  linux.Runner
	systemd systemd.Client
	health  func(context.Context, config.Config) Check
}

// New 使用注入的主机依赖创建应用编排器。
func New(cfg config.Config, runner linux.Runner) Application {
	return NewWithHealth(cfg, runner, mihomoHealthCheck)
}

// NewWithHealth 创建可替换 Mihomo 健康检查的应用编排器。
func NewWithHealth(cfg config.Config, runner linux.Runner, health func(context.Context, config.Config) Check) Application {
	return Application{config: cfg, runner: runner, systemd: systemd.NewClient(runner), health: health}
}

// MihomoRestart 重载 unit 定义并重启 Mihomo。
func (a Application) MihomoRestart(ctx context.Context) Result {
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return result("mihomo restart", []Check{failed("systemd reload", err.Error())})
	}
	if err := a.systemd.Restart(ctx, a.config.MihomoUnit); err != nil {
		return result("mihomo restart", []Check{failed("mihomo restart", err.Error())})
	}
	return a.mihomoPostRestart(ctx, "mihomo restart", "", "restart")
}

// MihomoApply 校验、快照、重启并确认 Mihomo 健康状态。
func (a Application) MihomoApply(ctx context.Context, dryRun bool) Result {
	validation := a.ValidateMihomo(ctx)
	if !validation.OK {
		return validation
	}
	if dryRun {
		return result("mihomo apply", []Check{{Name: "plan", OK: true, Severity: "error", Detail: "将创建 mihomo 快照、重载 systemd 并重启服务"}})
	}
	manifest, err := snapshot.New(a.config.RootDir).Create(ctx, "mihomo")
	if err != nil {
		return result("mihomo apply", []Check{failed("snapshot", err.Error())})
	}
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return a.operationFailure(ctx, "mihomo apply", "mihomo", manifest.ID, "daemon-reload", err)
	}
	if err := a.systemd.Restart(ctx, a.config.MihomoUnit); err != nil {
		return a.operationFailure(ctx, "mihomo apply", "mihomo", manifest.ID, "restart", err)
	}
	return a.mihomoPostRestart(ctx, "mihomo apply", manifest.ID, "apply")
}

// MihomoRollback 创建当前保护快照后恢复指定的 Mihomo 快照。
func (a Application) MihomoRollback(ctx context.Context, id string, dryRun bool) Result {
	store := snapshot.New(a.config.RootDir)
	if _, err := store.Verify(ctx, id); err != nil {
		return result("mihomo rollback", []Check{failed("snapshot", err.Error())})
	}
	if dryRun {
		return result("mihomo rollback", []Check{{Name: "plan", OK: true, Severity: "error", Detail: "将保护当前配置并恢复快照 " + id}})
	}
	protection, err := store.Create(ctx, "mihomo")
	if err != nil {
		return result("mihomo rollback", []Check{failed("protection snapshot", err.Error())})
	}
	if _, err := store.Restore(ctx, id, "mihomo"); err != nil {
		return a.operationFailure(ctx, "mihomo rollback", "mihomo", protection.ID, "restore", err)
	}
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return a.operationFailure(ctx, "mihomo rollback", "mihomo", protection.ID, "daemon-reload", err)
	}
	if err := a.systemd.Restart(ctx, a.config.MihomoUnit); err != nil {
		return a.operationFailure(ctx, "mihomo rollback", "mihomo", protection.ID, "restart", err)
	}
	return a.mihomoPostRestart(ctx, "mihomo rollback", protection.ID, "rollback")
}

// EasyTierStatus 返回 EasyTier 专项状态和网络健康信息。
func (a Application) EasyTierStatus(ctx context.Context) Result {
	checks := []Check{a.serviceStateCheck(ctx, "easytier", a.config.EasyTierUnit), a.serviceEnabledCheck(ctx, "easytier", a.config.EasyTierUnit), a.fileCheck("easytier configuration", a.config.EasyTierConfig), a.easyTierPeerCheck(ctx), a.easyTierInterfaceCheck(ctx)}
	return result("easytier status", checks)
}

// ValidateEasyTier 使用 TOML 语法解析校验权威 EasyTier 配置。
func (a Application) ValidateEasyTier(ctx context.Context) Result {
	if err := ctx.Err(); err != nil {
		return result("easytier validate", []Check{failed("easytier configuration", err.Error())})
	}
	data, err := os.ReadFile(a.config.EasyTierSourceFile())
	if err != nil {
		return result("easytier validate", []Check{failed("easytier configuration", err.Error())})
	}
	var parsed map[string]any
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return result("easytier validate", []Check{failed("easytier configuration", err.Error())})
	}
	return result("easytier validate", []Check{{Name: "easytier configuration", OK: true, Severity: "error", Detail: "TOML 语法有效；未启动额外 EasyTier 实例"}})
}

// EasyTierRestart 重载 unit 定义并重启 EasyTier。
func (a Application) EasyTierRestart(ctx context.Context) Result {
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return result("easytier restart", []Check{failed("systemd reload", err.Error())})
	}
	if err := a.systemd.Restart(ctx, a.config.EasyTierUnit); err != nil {
		return result("easytier restart", []Check{failed("easytier restart", err.Error())})
	}
	return a.easyTierPostRestart(ctx, "easytier restart", "", "restart")
}

// EasyTierApply 校验、快照、重启并确认 EasyTier 健康状态。
func (a Application) EasyTierApply(ctx context.Context, dryRun bool) Result {
	if validation := a.ValidateEasyTier(ctx); !validation.OK {
		return validation
	}
	if dryRun {
		return result("easytier apply", []Check{{Name: "plan", OK: true, Severity: "error", Detail: "将创建 easytier 快照、重载 systemd 并重启服务"}})
	}
	manifest, err := snapshot.New(a.config.RootDir).Create(ctx, "easytier")
	if err != nil {
		return result("easytier apply", []Check{failed("snapshot", err.Error())})
	}
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return a.operationFailure(ctx, "easytier apply", "easytier", manifest.ID, "daemon-reload", err)
	}
	if err := a.systemd.Restart(ctx, a.config.EasyTierUnit); err != nil {
		return a.operationFailure(ctx, "easytier apply", "easytier", manifest.ID, "restart", err)
	}
	return a.easyTierPostRestart(ctx, "easytier apply", manifest.ID, "apply")
}

// EasyTierRollback 创建保护快照后恢复指定 EasyTier 快照。
func (a Application) EasyTierRollback(ctx context.Context, id string, dryRun bool) Result {
	store := snapshot.New(a.config.RootDir)
	if _, err := store.Verify(ctx, id); err != nil {
		return result("easytier rollback", []Check{failed("snapshot", err.Error())})
	}
	if dryRun {
		return result("easytier rollback", []Check{{Name: "plan", OK: true, Severity: "error", Detail: "将保护当前配置并恢复快照 " + id}})
	}
	protection, err := store.Create(ctx, "easytier")
	if err != nil {
		return result("easytier rollback", []Check{failed("protection snapshot", err.Error())})
	}
	if _, err := store.Restore(ctx, id, "easytier"); err != nil {
		return a.operationFailure(ctx, "easytier rollback", "easytier", protection.ID, "restore", err)
	}
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return a.operationFailure(ctx, "easytier rollback", "easytier", protection.ID, "daemon-reload", err)
	}
	if err := a.systemd.Restart(ctx, a.config.EasyTierUnit); err != nil {
		return a.operationFailure(ctx, "easytier rollback", "easytier", protection.ID, "restart", err)
	}
	return a.easyTierPostRestart(ctx, "easytier rollback", protection.ID, "rollback")
}

func (a Application) easyTierPostRestart(ctx context.Context, operation, snapshotID, action string) Result {
	state, err := a.systemd.State(ctx, a.config.EasyTierUnit)
	if err != nil || state != "active" {
		if err != nil {
			return a.operationFailure(ctx, operation, "easytier", snapshotID, action, err)
		}
		return a.operationFailure(ctx, operation, "easytier", snapshotID, action, fmt.Errorf("服务未处于 active 状态"))
	}
	peer := a.easyTierPeerHealthCheck(ctx)
	if !peer.OK {
		return a.operationFailure(ctx, operation, "easytier", snapshotID, action, fmt.Errorf("健康检查失败: %s", peer.Detail))
	}
	if err := audit.Append(ctx, a.config.RootDir, audit.Event{Operation: operation, Target: "easytier", Snapshot: snapshotID, Result: "success"}); err != nil {
		return result(operation, []Check{failed("audit", err.Error())})
	}
	return result(operation, []Check{{Name: "easytier service", OK: true, Severity: "error", Detail: state}, peer, a.easyTierInterfaceCheck(ctx)})
}

func (a Application) mihomoPostRestart(ctx context.Context, operation, snapshotID, action string) Result {
	state, err := a.systemd.State(ctx, a.config.MihomoUnit)
	if err != nil || state != "active" {
		detail := "服务未处于 active 状态"
		if err != nil {
			detail = err.Error()
		}
		return a.operationFailure(ctx, operation, "mihomo", snapshotID, action, fmt.Errorf("%s", detail))
	}
	health := a.health(ctx, a.config)
	if !health.OK {
		return a.operationFailure(ctx, operation, "mihomo", snapshotID, action, fmt.Errorf("健康检查失败: %s", health.Detail))
	}
	if err := audit.Append(ctx, a.config.RootDir, audit.Event{Operation: operation, Target: "mihomo", Snapshot: snapshotID, Result: "success"}); err != nil {
		return result(operation, []Check{failed("audit", err.Error())})
	}
	return result(operation, []Check{{Name: "mihomo service", OK: true, Severity: "error", Detail: state}, health})
}

func (a Application) operationFailure(ctx context.Context, operation, target, snapshotID, step string, cause error) Result {
	detail := step + ": " + cause.Error()
	if err := audit.Append(ctx, a.config.RootDir, audit.Event{Operation: operation, Target: target, Snapshot: snapshotID, Result: "failed", Details: map[string]string{"step": step, "error": cause.Error()}}); err != nil {
		detail += "；审计记录失败: " + err.Error()
	}
	if snapshotID != "" {
		detail += "；可使用 netkit " + target + " rollback " + snapshotID
	}
	return result(operation, []Check{failed(operation, detail)})
}

func mihomoHealthCheck(ctx context.Context, cfg config.Config) Check {
	proxyURL, err := url.Parse(cfg.MihomoProxyURL)
	if err != nil {
		return failed("mihomo proxy health", err.Error())
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, cfg.MihomoHealthURL, nil)
	if err != nil {
		return failed("mihomo proxy health", err.Error())
	}
	response, err := client.Do(request)
	if err != nil {
		return failed("mihomo proxy health", err.Error())
	}
	defer response.Body.Close()
	ok := response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
	return Check{Name: "mihomo proxy health", OK: ok, Severity: "error", Detail: response.Status}
}

// Status 并发检查受管服务的 systemd、端口和配置状态。
func (a Application) Status(ctx context.Context) Result {
	checks := make([]Check, 6)
	var group sync.WaitGroup
	checksToRun := []func() Check{
		func() Check { return a.serviceStateCheck(ctx, "mihomo", a.config.MihomoUnit) },
		func() Check { return a.serviceEnabledCheck(ctx, "mihomo", a.config.MihomoUnit) },
		func() Check { return a.serviceStateCheck(ctx, "easytier", a.config.EasyTierUnit) },
		func() Check { return a.serviceEnabledCheck(ctx, "easytier", a.config.EasyTierUnit) },
		func() Check { return a.mihomoPortCheck(ctx) },
		func() Check { return a.fileCheck("easytier configuration", a.config.EasyTierConfig) },
	}
	for index, check := range checksToRun {
		group.Add(1)
		go func(index int, check func() Check) {
			defer group.Done()
			checks[index] = check()
		}(index, check)
	}
	group.Wait()
	return result("status", checks)
}

func (a Application) serviceStateCheck(ctx context.Context, name, unit string) Check {
	state, err := a.systemd.State(ctx, unit)
	if err != nil {
		return failed(name+" service", err.Error())
	}
	return Check{Name: name + " service", OK: state == "active", Severity: "error", Detail: state}
}

func (a Application) serviceEnabledCheck(ctx context.Context, name, unit string) Check {
	enabled, err := a.systemd.Enabled(ctx, unit)
	if err != nil {
		return failed(name+" enabled", err.Error())
	}
	return Check{Name: name + " enabled", OK: enabled, Severity: "error", Detail: fmt.Sprintf("enabled=%t", enabled)}
}

// VerifyLinks 检查 Linux 标准入口是否解析到 Netkit 权威资产。
func (a Application) VerifyLinks(_ context.Context) Result {
	links := []struct{ name, path, want string }{
		{"mihomo config", "/etc/mihomo", a.config.MihomoSourceDir()},
		{"easytier config", "/etc/easytier", filepath.Dir(a.config.EasyTierSourceFile())},
	}
	mihomoService, _ := a.config.ServiceFile("mihomo")
	easytierService, _ := a.config.ServiceFile("easytier")
	links = append(links,
		struct{ name, path, want string }{"mihomo unit", "/etc/systemd/system/mihomo.service", mihomoService},
		struct{ name, path, want string }{"easytier unit", "/etc/systemd/system/easytier.service", easytierService},
	)
	checks := make([]Check, 0, len(links))
	for _, link := range links {
		resolved, err := filepath.EvalSymlinks(link.path)
		if err != nil {
			checks = append(checks, failed(link.name+" link", err.Error()))
			continue
		}
		checks = append(checks, Check{Name: link.name + " link", OK: resolved == link.want, Severity: "error", Detail: resolved})
	}
	return result("links verify", checks)
}

// ValidateMihomo 在不修改配置的前提下校验 Mihomo 当前配置。
func (a Application) ValidateMihomo(ctx context.Context) Result {
	commandResult, err := a.runner.Run(ctx, a.config.MihomoBinary, "-t", "-d", a.config.MihomoConfig)
	if err != nil {
		return result("mihomo validate", []Check{failed("mihomo configuration", err.Error())})
	}
	detail := commandResult.Stdout
	if commandResult.Stderr != "" {
		detail = commandResult.Stderr
	}
	return result("mihomo validate", []Check{{Name: "mihomo configuration", OK: commandResult.ExitCode == 0, Severity: "error", Detail: detail}})
}

// Doctor 执行相互独立的主机、链接、服务和网络检查。
func (a Application) Doctor(ctx context.Context) Result {
	checks := []Check{a.rootCheck(), a.fileCheck("mihomo binary", a.config.MihomoBinary), a.fileCheck("easytier CLI", a.config.EasyTierCLI)}
	status := a.Status(ctx)
	checks = append(checks, status.Checks...)
	links := a.VerifyLinks(ctx)
	checks = append(checks, links.Checks...)
	checks = append(checks, a.ValidateMihomo(ctx).Checks...)
	checks = append(checks, a.easyTierPeerCheck(ctx), a.easyTierInterfaceCheck(ctx), a.dockerProxyCheck(ctx))
	return result("doctor", checks)
}

// EasyTierPeers 查询 EasyTier 当前 peer 信息。
func (a Application) EasyTierPeers(ctx context.Context) Result {
	return result("easytier peers", []Check{a.easyTierPeerCheck(ctx)})
}

// PingEasyTierPeer 测试到指定 EasyTier 节点的 ICMP 连通性。
func (a Application) PingEasyTierPeer(ctx context.Context, address string) Result {
	if net.ParseIP(address) == nil {
		return result("easytier ping", []Check{failed("easytier peer", "无效的 IP 地址")})
	}
	commandResult, err := a.runner.Run(ctx, "ping", "-c", "3", "-W", "2", address)
	if err != nil {
		return result("easytier ping", []Check{failed("easytier peer", err.Error())})
	}
	detail := commandResult.Stdout
	if commandResult.Stderr != "" {
		detail = commandResult.Stderr
	}
	return result("easytier ping", []Check{{Name: "easytier peer", OK: commandResult.ExitCode == 0, Severity: "error", Detail: detail}})
}

// Logs 读取指定受管服务的 systemd 日志。
func (a Application) Logs(ctx context.Context, target string, since time.Duration, follow bool) Result {
	unit, err := a.unitFor(target)
	if err != nil {
		return result(target+" logs", []Check{failed(target+" logs", err.Error())})
	}
	commandResult, err := a.systemd.Logs(ctx, unit, since, follow)
	if err != nil {
		return result(target+" logs", []Check{failed(target+" logs", err.Error())})
	}
	detail := commandResult.Stdout
	if commandResult.Stderr != "" {
		detail = commandResult.Stderr
	}
	return result(target+" logs", []Check{{Name: target + " logs", OK: commandResult.ExitCode == 0, Severity: "error", Detail: detail}})
}

func (a Application) unitFor(target string) (string, error) {
	switch target {
	case "mihomo":
		return a.config.MihomoUnit, nil
	case "easytier":
		return a.config.EasyTierUnit, nil
	default:
		return "", fmt.Errorf("不支持的服务 %q", target)
	}
}

func (a Application) rootCheck() Check {
	if os.Geteuid() == 0 {
		return Check{Name: "root permission", OK: true, Severity: "error", Detail: "running as root"}
	}
	return failed("root permission", "must run as root")
}

func (a Application) fileCheck(name, path string) Check {
	if _, err := os.Stat(path); err != nil {
		return failed(name, err.Error())
	}
	return Check{Name: name, OK: true, Severity: "error", Detail: path}
}

func (a Application) mihomoPortCheck(ctx context.Context) Check {
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:7890")
	if err != nil {
		return failed("mihomo proxy port", err.Error())
	}
	if err := connection.Close(); err != nil {
		return failed("mihomo proxy port", err.Error())
	}
	return Check{Name: "mihomo proxy port", OK: true, Severity: "error", Detail: "127.0.0.1:7890 reachable"}
}

func (a Application) easyTierPeerCheck(ctx context.Context) Check {
	commandResult, err := a.runner.Run(ctx, a.config.EasyTierCLI, "peer")
	if err != nil {
		return Check{Name: "easytier peers", OK: false, Severity: "warning", Detail: err.Error()}
	}
	if commandResult.ExitCode != 0 {
		return Check{Name: "easytier peers", OK: false, Severity: "warning", Detail: commandResult.Stderr}
	}
	return Check{Name: "easytier peers", OK: true, Severity: "warning", Detail: commandResult.Stdout}
}

func (a Application) easyTierPeerHealthCheck(ctx context.Context) Check {
	commandResult, err := a.runner.Run(ctx, a.config.EasyTierCLI, "peer")
	if err != nil {
		return failed("easytier peers", err.Error())
	}
	if commandResult.ExitCode != 0 {
		return failed("easytier peers", commandResult.Stderr)
	}
	return Check{Name: "easytier peers", OK: true, Severity: "error", Detail: commandResult.Stdout}
}

func (a Application) easyTierInterfaceCheck(ctx context.Context) Check {
	commandResult, err := a.runner.Run(ctx, "ip", "-o", "-4", "addr", "show")
	if err != nil {
		return Check{Name: "easytier interface", OK: false, Severity: "warning", Detail: err.Error()}
	}
	if commandResult.ExitCode != 0 {
		return Check{Name: "easytier interface", OK: false, Severity: "warning", Detail: commandResult.Stderr}
	}
	if strings.Contains(commandResult.Stdout, "10.126.126.") {
		return Check{Name: "easytier interface", OK: true, Severity: "warning", Detail: "检测到 10.126.126.0/24 虚拟地址"}
	}
	return Check{Name: "easytier interface", OK: false, Severity: "warning", Detail: "未检测到 10.126.126.0/24 虚拟地址"}
}

func (a Application) dockerProxyCheck(ctx context.Context) Check {
	commandResult, err := a.runner.Run(ctx, "systemctl", "show", "docker", "--property", "Environment", "--value")
	if err != nil {
		return Check{Name: "docker proxy", OK: false, Severity: "warning", Detail: err.Error()}
	}
	if commandResult.ExitCode != 0 {
		return Check{Name: "docker proxy", OK: false, Severity: "warning", Detail: commandResult.Stderr}
	}
	if strings.Contains(commandResult.Stdout, "HTTP_PROXY=http://127.0.0.1:7890") {
		return Check{Name: "docker proxy", OK: true, Severity: "warning", Detail: "Docker 已使用 Mihomo 代理"}
	}
	return Check{Name: "docker proxy", OK: false, Severity: "warning", Detail: "Docker 未检测到 http://127.0.0.1:7890 代理"}
}

func failed(name, detail string) Check {
	return Check{Name: name, OK: false, Severity: "error", Detail: detail}
}

func result(operation string, checks []Check) Result {
	ok := true
	for _, check := range checks {
		if !check.OK && check.Severity == "error" {
			ok = false
		}
	}
	return Result{OK: ok, Operation: operation, Checks: checks}
}
