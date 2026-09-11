package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rustyllh/netkit/internal/config"
)

// Status 并发检查受管服务的 systemd、端口和配置状态。
func (a Application) Status(ctx context.Context) Result {
	checks := make([]Check, 6)
	var group sync.WaitGroup
	checksToRun := []func() Check{
		func() Check { return a.serviceStateCheck(ctx, config.ServiceMihomo, a.config.Mihomo.Unit) },
		func() Check { return a.serviceEnabledCheck(ctx, config.ServiceMihomo, a.config.Mihomo.Unit) },
		func() Check { return a.serviceStateCheck(ctx, config.ServiceEasyTier, a.config.EasyTier.Unit) },
		func() Check { return a.serviceEnabledCheck(ctx, config.ServiceEasyTier, a.config.EasyTier.Unit) },
		func() Check { return a.mihomoPortCheck(ctx) },
		func() Check { return a.fileCheck("easytier configuration", a.config.EasyTier.ConfigPath) },
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

// VerifyLinks 检查 Linux 标准入口是否解析到 Netkit 权威资产。
func (a Application) VerifyLinks(_ context.Context) Result {
	links := []struct{ name, path, want string }{
		{"mihomo config", a.config.Mihomo.ConfigPath, a.config.MihomoSourceDir()},
		{"easytier config", filepath.Dir(a.config.EasyTier.ConfigPath), filepath.Dir(a.config.EasyTierSourceFile())},
	}
	mihomoService, _ := a.config.ServiceFile(config.ServiceMihomo)
	easyTierService, _ := a.config.ServiceFile(config.ServiceEasyTier)
	links = append(links,
		struct{ name, path, want string }{"mihomo unit", filepath.Join("/etc/systemd/system", a.config.Mihomo.Unit), mihomoService},
		struct{ name, path, want string }{"easytier unit", filepath.Join("/etc/systemd/system", a.config.EasyTier.Unit), easyTierService},
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

// Logs 读取指定受管服务的 systemd 日志。
func (a Application) Logs(ctx context.Context, target string, lines int, since time.Duration) Result {
	unit, err := a.unitFor(target)
	if err != nil {
		return result(target+" logs", []Check{failed(target+" logs", err.Error())})
	}
	commandResult, err := a.systemd.Logs(ctx, unit, lines, since)
	if err != nil {
		return result(target+" logs", []Check{failed(target+" logs", err.Error())})
	}
	detail := commandResult.Stdout
	if commandResult.Stderr != "" {
		detail = commandResult.Stderr
	}
	return result(target+" logs", []Check{{Name: target + " logs", OK: commandResult.ExitCode == 0, Severity: "error", Detail: detail}})
}

// FollowLogs 先输出最近日志，再将指定受管服务的新增日志实时转发到输出流。
func (a Application) FollowLogs(ctx context.Context, target string, lines int, since time.Duration, stdout, stderr io.Writer) error {
	unit, err := a.unitFor(target)
	if err != nil {
		return fmt.Errorf("持续读取 %s 日志: %w", target, err)
	}
	if err := a.systemd.FollowLogs(ctx, unit, lines, since, stdout, stderr); err != nil {
		return fmt.Errorf("持续读取 %s 日志: %w", target, err)
	}
	return nil
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

func (a Application) unitFor(target string) (string, error) {
	service, err := a.config.Service(target)
	if err != nil {
		return "", fmt.Errorf("不支持的服务 %q", target)
	}
	return service.Unit, nil
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
