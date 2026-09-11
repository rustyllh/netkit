package app

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/rustyllh/netkit/internal/audit"
	"github.com/rustyllh/netkit/internal/config"
	"github.com/rustyllh/netkit/internal/snapshot"
)

// MihomoRestart 重载 unit 定义并重启 Mihomo。
func (a Application) MihomoRestart(ctx context.Context) Result {
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return result("mihomo restart", []Check{failed("systemd reload", err.Error())})
	}
	if err := a.systemd.Restart(ctx, a.config.Mihomo.Unit); err != nil {
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
	manifest, err := snapshot.New(a.config).Create(ctx, config.ServiceMihomo)
	if err != nil {
		return result("mihomo apply", []Check{failed("snapshot", err.Error())})
	}
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return a.operationFailure(ctx, "mihomo apply", config.ServiceMihomo, manifest.ID, "daemon-reload", err)
	}
	if err := a.systemd.Restart(ctx, a.config.Mihomo.Unit); err != nil {
		return a.operationFailure(ctx, "mihomo apply", config.ServiceMihomo, manifest.ID, "restart", err)
	}
	return a.mihomoPostRestart(ctx, "mihomo apply", manifest.ID, "apply")
}

// MihomoRollback 创建当前保护快照后恢复指定的 Mihomo 快照。
func (a Application) MihomoRollback(ctx context.Context, id string, dryRun bool) Result {
	store := snapshot.New(a.config)
	if _, err := store.Verify(ctx, id); err != nil {
		return result("mihomo rollback", []Check{failed("snapshot", err.Error())})
	}
	if dryRun {
		return result("mihomo rollback", []Check{{Name: "plan", OK: true, Severity: "error", Detail: "将保护当前配置并恢复快照 " + id}})
	}
	protection, err := store.Create(ctx, config.ServiceMihomo)
	if err != nil {
		return result("mihomo rollback", []Check{failed("protection snapshot", err.Error())})
	}
	if _, err := store.Restore(ctx, id, config.ServiceMihomo); err != nil {
		return a.operationFailure(ctx, "mihomo rollback", config.ServiceMihomo, protection.ID, "restore", err)
	}
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return a.operationFailure(ctx, "mihomo rollback", config.ServiceMihomo, protection.ID, "daemon-reload", err)
	}
	if err := a.systemd.Restart(ctx, a.config.Mihomo.Unit); err != nil {
		return a.operationFailure(ctx, "mihomo rollback", config.ServiceMihomo, protection.ID, "restart", err)
	}
	return a.mihomoPostRestart(ctx, "mihomo rollback", protection.ID, "rollback")
}

func (a Application) mihomoPostRestart(ctx context.Context, operation, snapshotID, action string) Result {
	state, err := a.systemd.State(ctx, a.config.Mihomo.Unit)
	if err != nil || state != "active" {
		detail := "服务未处于 active 状态"
		if err != nil {
			detail = err.Error()
		}
		return a.operationFailure(ctx, operation, config.ServiceMihomo, snapshotID, action, fmt.Errorf("%s", detail))
	}
	health := a.waitForCheck(ctx, func(ctx context.Context) Check {
		return a.health(ctx, a.config)
	})
	if !health.OK {
		return a.operationFailure(ctx, operation, config.ServiceMihomo, snapshotID, action, fmt.Errorf("健康检查失败: %s", health.Detail))
	}
	if err := audit.Append(ctx, a.config.RootDir, audit.Event{Operation: operation, Target: config.ServiceMihomo, Snapshot: snapshotID, Result: "success"}); err != nil {
		return result(operation, []Check{failed("audit", err.Error())})
	}
	return result(operation, []Check{{Name: "mihomo service", OK: true, Severity: "error", Detail: state}, health})
}

// ValidateMihomo 在不修改配置的前提下校验 Mihomo 当前配置。
func (a Application) ValidateMihomo(ctx context.Context) Result {
	commandResult, err := a.runner.Run(ctx, a.config.MihomoBinary, "-t", "-d", a.config.Mihomo.ConfigPath)
	if err != nil {
		return result("mihomo validate", []Check{failed("mihomo configuration", err.Error())})
	}
	detail := commandResult.Stdout
	if commandResult.Stderr != "" {
		detail = commandResult.Stderr
	}
	return result("mihomo validate", []Check{{Name: "mihomo configuration", OK: commandResult.ExitCode == 0, Severity: "error", Detail: detail}})
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
