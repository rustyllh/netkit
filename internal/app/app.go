// Package app 包含与命令行无关的 Netkit 用例编排。
package app

import (
	"context"

	"github.com/rustyllh/netkit/internal/config"
	"github.com/rustyllh/netkit/internal/linux"
	"github.com/rustyllh/netkit/internal/systemd"
)

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
