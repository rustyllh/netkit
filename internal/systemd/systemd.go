// Package systemd 封装 Netkit 所需的少量 systemctl 操作。
package systemd

import (
	"context"
	"fmt"
	"time"

	"github.com/rustyllh/netkit/internal/linux"
)

// Client 通过进程执行器读取 systemd unit 状态。
type Client struct{ runner linux.Runner }

// NewClient 创建 systemd 客户端。
func NewClient(runner linux.Runner) Client { return Client{runner: runner} }

// State 返回 unit 在 systemd 中的活动状态。
func (c Client) State(ctx context.Context, unit string) (string, error) {
	result, err := c.runner.Run(ctx, "systemctl", "is-active", unit)
	if err != nil {
		return "unknown", fmt.Errorf("read %s state: %w", unit, err)
	}
	if result.Stdout != "" {
		return result.Stdout, nil
	}
	if result.ExitCode != 0 {
		return "inactive", nil
	}
	return "unknown", nil
}

// Enabled 返回 unit 是否已设置为开机启动。
func (c Client) Enabled(ctx context.Context, unit string) (bool, error) {
	result, err := c.runner.Run(ctx, "systemctl", "is-enabled", unit)
	if err != nil {
		return false, fmt.Errorf("read %s enabled state: %w", unit, err)
	}
	return result.ExitCode == 0 && result.Stdout == "enabled", nil
}

// DaemonReload 重新加载 systemd unit 定义。
func (c Client) DaemonReload(ctx context.Context) error {
	result, err := c.runner.Run(ctx, "systemctl", "daemon-reload")
	if err != nil {
		return fmt.Errorf("重新加载 systemd: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("重新加载 systemd 失败: %s", result.Stderr)
	}
	return nil
}

// Restart 重启指定 unit。
func (c Client) Restart(ctx context.Context, unit string) error {
	result, err := c.runner.Run(ctx, "systemctl", "restart", unit)
	if err != nil {
		return fmt.Errorf("重启 %s: %w", unit, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("重启 %s 失败: %s", unit, result.Stderr)
	}
	return nil
}

// Logs 读取 unit 的 journald 日志。
func (c Client) Logs(ctx context.Context, unit string, since time.Duration, follow bool) (linux.Result, error) {
	if since <= 0 {
		return linux.Result{}, fmt.Errorf("日志时间范围必须为正数")
	}
	args := []string{"--unit", unit, "--no-pager", "--output", "short-iso", "--since", since.String() + " ago"}
	if follow {
		args = append(args, "--follow")
	}
	result, err := c.runner.Run(ctx, "journalctl", args...)
	if err != nil {
		return linux.Result{}, fmt.Errorf("读取 %s 日志: %w", unit, err)
	}
	return result, nil
}
