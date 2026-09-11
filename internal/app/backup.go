package app

import (
	"context"

	"github.com/rustyllh/netkit/internal/audit"
	"github.com/rustyllh/netkit/internal/snapshot"
)

// CreateBackup 创建指定网络配置的可验证快照并写入审计事件。
func (a Application) CreateBackup(ctx context.Context, target string) Result {
	manifest, err := snapshot.New(a.config).Create(ctx, target)
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
	manifests, err := snapshot.New(a.config).List(ctx)
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
	manifest, err := snapshot.New(a.config).Verify(ctx, id)
	if err != nil {
		return result("backup verify", []Check{failed("backup", err.Error())})
	}
	if err := audit.Append(ctx, a.config.RootDir, audit.Event{Operation: "backup verify", Target: manifest.Target, Snapshot: id, Result: "success"}); err != nil {
		return result("backup verify", []Check{failed("audit", err.Error())})
	}
	return result("backup verify", []Check{{Name: "backup", OK: true, Severity: "error", Detail: id}})
}
