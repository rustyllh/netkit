package app

import (
	"context"
	"time"

	"github.com/rustyllh/netkit/internal/audit"
)

const healthCheckRetryInterval = 200 * time.Millisecond

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

func (a Application) waitForCheck(ctx context.Context, check func(context.Context) Check) Check {
	waitCtx, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()
	return waitForCheck(waitCtx, healthCheckRetryInterval, check)
}

// waitForCheck 在调用方的超时时间内等待服务通过健康检查。
func waitForCheck(ctx context.Context, interval time.Duration, check func(context.Context) Check) Check {
	last := check(ctx)
	if last.OK {
		return last
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			last.Detail += "；等待服务就绪超时: " + ctx.Err().Error()
			return last
		case <-ticker.C:
			last = check(ctx)
			if last.OK {
				return last
			}
		}
	}
}
