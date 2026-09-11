// Package app 包含与命令行无关的 Netkit 用例编排。
package app

import (
	"context"

	"github.com/rustyllh/netkit/internal/components"
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

// InstallComponents 安装官方 Mihomo 与 EasyTier 二进制，不启动服务。
func (a Application) InstallComponents(ctx context.Context, request components.Request) Result {
	installed, err := components.New(a.config.RootDir).Install(ctx, request)
	if err != nil {
		return result("components install", []Check{failed("components install", err.Error())})
	}
	checks := make([]Check, 0, len(installed))
	for _, item := range installed {
		checks = append(checks, Check{
			Name:     item.Name + " installed",
			OK:       true,
			Severity: "error",
			Detail:   item.Version + " " + item.Path,
		})
	}
	return result("components install", checks)
}

// New 使用注入的主机依赖创建应用编排器。
func New(cfg config.Config, runner linux.Runner) Application {
	return NewWithHealth(cfg, runner, mihomoHealthCheck)
}

// NewWithHealth 创建可替换 Mihomo 健康检查的应用编排器。
func NewWithHealth(cfg config.Config, runner linux.Runner, health func(context.Context, config.Config) Check) Application {
	return Application{config: cfg, runner: runner, systemd: systemd.NewClient(runner), health: health}
}
