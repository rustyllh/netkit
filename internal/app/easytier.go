package app

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/pelletier/go-toml/v2"
	"github.com/rustyllh/netkit/internal/audit"
	"github.com/rustyllh/netkit/internal/config"
	"github.com/rustyllh/netkit/internal/snapshot"
)

// EasyTierStatus 返回 EasyTier 专项状态和网络健康信息。
func (a Application) EasyTierStatus(ctx context.Context) Result {
	checks := []Check{a.serviceStateCheck(ctx, config.ServiceEasyTier, a.config.EasyTier.Unit), a.serviceEnabledCheck(ctx, config.ServiceEasyTier, a.config.EasyTier.Unit), a.fileCheck("easytier configuration", a.config.EasyTier.ConfigPath), a.easyTierPeerCheck(ctx), a.easyTierInterfaceCheck(ctx)}
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
	if err := a.systemd.Restart(ctx, a.config.EasyTier.Unit); err != nil {
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
	manifest, err := snapshot.New(a.config).Create(ctx, config.ServiceEasyTier)
	if err != nil {
		return result("easytier apply", []Check{failed("snapshot", err.Error())})
	}
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return a.operationFailure(ctx, "easytier apply", config.ServiceEasyTier, manifest.ID, "daemon-reload", err)
	}
	if err := a.systemd.Restart(ctx, a.config.EasyTier.Unit); err != nil {
		return a.operationFailure(ctx, "easytier apply", config.ServiceEasyTier, manifest.ID, "restart", err)
	}
	return a.easyTierPostRestart(ctx, "easytier apply", manifest.ID, "apply")
}

// EasyTierRollback 创建保护快照后恢复指定 EasyTier 快照。
func (a Application) EasyTierRollback(ctx context.Context, id string, dryRun bool) Result {
	store := snapshot.New(a.config)
	if _, err := store.Verify(ctx, id); err != nil {
		return result("easytier rollback", []Check{failed("snapshot", err.Error())})
	}
	if dryRun {
		return result("easytier rollback", []Check{{Name: "plan", OK: true, Severity: "error", Detail: "将保护当前配置并恢复快照 " + id}})
	}
	protection, err := store.Create(ctx, config.ServiceEasyTier)
	if err != nil {
		return result("easytier rollback", []Check{failed("protection snapshot", err.Error())})
	}
	if _, err := store.Restore(ctx, id, config.ServiceEasyTier); err != nil {
		return a.operationFailure(ctx, "easytier rollback", config.ServiceEasyTier, protection.ID, "restore", err)
	}
	if err := a.systemd.DaemonReload(ctx); err != nil {
		return a.operationFailure(ctx, "easytier rollback", config.ServiceEasyTier, protection.ID, "daemon-reload", err)
	}
	if err := a.systemd.Restart(ctx, a.config.EasyTier.Unit); err != nil {
		return a.operationFailure(ctx, "easytier rollback", config.ServiceEasyTier, protection.ID, "restart", err)
	}
	return a.easyTierPostRestart(ctx, "easytier rollback", protection.ID, "rollback")
}

func (a Application) easyTierPostRestart(ctx context.Context, operation, snapshotID, action string) Result {
	state, err := a.systemd.State(ctx, a.config.EasyTier.Unit)
	if err != nil || state != "active" {
		if err != nil {
			return a.operationFailure(ctx, operation, config.ServiceEasyTier, snapshotID, action, err)
		}
		return a.operationFailure(ctx, operation, config.ServiceEasyTier, snapshotID, action, fmt.Errorf("服务未处于 active 状态"))
	}
	peer := a.waitForCheck(ctx, a.easyTierPeerHealthCheck)
	if !peer.OK {
		return a.operationFailure(ctx, operation, config.ServiceEasyTier, snapshotID, action, fmt.Errorf("健康检查失败: %s", peer.Detail))
	}
	if err := audit.Append(ctx, a.config.RootDir, audit.Event{Operation: operation, Target: config.ServiceEasyTier, Snapshot: snapshotID, Result: "success"}); err != nil {
		return result(operation, []Check{failed("audit", err.Error())})
	}
	return result(operation, []Check{{Name: "easytier service", OK: true, Severity: "error", Detail: state}, peer, a.easyTierInterfaceCheck(ctx)})
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
