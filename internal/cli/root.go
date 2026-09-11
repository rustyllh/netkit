// Package cli 定义 Netkit 的命令行接口。
package cli

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/rustyllh/netkit/internal/app"
	"github.com/rustyllh/netkit/internal/components"
	"github.com/spf13/cobra"
)

// NewRootCommand 构建 netkit 命令树。
func NewRootCommand() *cobra.Command {
	var rootDir string
	var timeout time.Duration
	var jsonOutput bool
	command := &cobra.Command{
		Use:           "netkit",
		Short:         "Controlled Linux operations for Mihomo and EasyTier",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	command.CompletionOptions.DisableDefaultCmd = true
	command.PersistentFlags().StringVar(&rootDir, "root", "", "Netkit asset root (default /root/netkit)")
	command.PersistentFlags().DurationVar(&timeout, "timeout", 30*time.Second, "external command timeout")
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit JSON results")
	runtime := newCommandRuntime(&rootDir, &timeout, &jsonOutput)

	command.AddCommand(newResultCommand(runtime, "status", "Show managed service status", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.Status(ctx)
	}))
	command.AddCommand(newLinksCommand(runtime))
	command.AddCommand(newMihomoCommand(runtime))
	command.AddCommand(newEasyTierCommand(runtime))
	command.AddCommand(newBackupCommand(runtime))
	command.AddCommand(newComponentsCommand(runtime))
	command.AddCommand(newResultCommand(runtime, "doctor", "Run host and network diagnostics", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.Doctor(ctx)
	}))
	return command
}

func newComponentsCommand(runtime commandRuntime) *cobra.Command {
	command := &cobra.Command{Use: "components", Short: "Install managed component binaries"}
	command.AddCommand(newComponentsInstallCommand(runtime))
	return command
}

func newComponentsInstallCommand(runtime commandRuntime) *cobra.Command {
	request := components.Request{}
	command := newResultCommand(runtime, "install", "Install Mihomo and EasyTier binaries", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.InstallComponents(ctx, request)
	})
	flags := command.Flags()
	flags.StringVar(&request.MihomoVersion, "mihomo-version", "", "Mihomo Release version; defaults to latest")
	flags.StringVar(&request.EasyTierVersion, "easytier-version", "", "EasyTier Release version; defaults to latest")
	flags.StringVar(&request.MihomoPackage, "mihomo-package", "", "path to a local Mihomo Release archive")
	flags.StringVar(&request.EasyTierPackage, "easytier-package", "", "path to a local EasyTier Release archive")
	return command
}

func newLinksCommand(runtime commandRuntime) *cobra.Command {
	command := &cobra.Command{Use: "links", Short: "Inspect standard Linux entry-point links"}
	command.AddCommand(newResultCommand(runtime, "verify", "Verify links resolve into the Netkit root", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.VerifyLinks(ctx)
	}))
	return command
}

func newMihomoCommand(runtime commandRuntime) *cobra.Command {
	command := &cobra.Command{Use: "mihomo", Short: "Operate on Mihomo"}
	command.AddCommand(newResultCommand(runtime, "validate", "Validate active Mihomo configuration", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.ValidateMihomo(ctx)
	}))
	command.AddCommand(newLogsCommand("mihomo", runtime))
	command.AddCommand(newResultCommand(runtime, "restart", "Restart Mihomo", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.MihomoRestart(ctx)
	}))
	command.AddCommand(newMihomoApplyCommand(runtime))
	command.AddCommand(newMihomoRollbackCommand(runtime))
	return command
}

func newEasyTierCommand(runtime commandRuntime) *cobra.Command {
	command := &cobra.Command{Use: "easytier", Short: "Operate on EasyTier"}
	command.AddCommand(newResultCommand(runtime, "status", "Show EasyTier status", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.EasyTierStatus(ctx)
	}))
	command.AddCommand(newResultCommand(runtime, "validate", "Validate EasyTier TOML configuration", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.ValidateEasyTier(ctx)
	}))
	command.AddCommand(newResultCommand(runtime, "peers", "Show EasyTier peers", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.EasyTierPeers(ctx)
	}))
	command.AddCommand(newPingCommand(runtime))
	command.AddCommand(newLogsCommand("easytier", runtime))
	command.AddCommand(newResultCommand(runtime, "restart", "Restart EasyTier", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.EasyTierRestart(ctx)
	}))
	command.AddCommand(newEasyTierApplyCommand(runtime))
	command.AddCommand(newEasyTierRollbackCommand(runtime))
	return command
}

func newBackupCommand(runtime commandRuntime) *cobra.Command {
	command := &cobra.Command{Use: "backup", Short: "Create and verify configuration snapshots"}
	command.AddCommand(newBackupCreateCommand(runtime))
	command.AddCommand(newResultCommand(runtime, "list", "List configuration snapshots", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.ListBackups(ctx)
	}))
	command.AddCommand(newBackupVerifyCommand(runtime))
	return command
}

func newPingCommand(runtime commandRuntime) *cobra.Command {
	return newArgumentCommand(runtime, "ping IP", "Ping an EasyTier peer", cobra.ExactArgs(1), func(ctx context.Context, application app.Application, args []string) app.Result {
		return application.PingEasyTierPeer(ctx, args[0])
	})
}

func newBackupCreateCommand(runtime commandRuntime) *cobra.Command {
	return newArgumentCommand(runtime, "create [mihomo|easytier|all]", "Create a configuration snapshot", cobra.MaximumNArgs(1), func(ctx context.Context, application app.Application, args []string) app.Result {
		target := "all"
		if len(args) == 1 {
			target = args[0]
		}
		return application.CreateBackup(ctx, target)
	})
}

func newBackupVerifyCommand(runtime commandRuntime) *cobra.Command {
	return newArgumentCommand(runtime, "verify SNAPSHOT_ID", "Verify a configuration snapshot", cobra.ExactArgs(1), func(ctx context.Context, application app.Application, args []string) app.Result {
		return application.VerifyBackup(ctx, args[0])
	})
}

type argumentOperation func(context.Context, app.Application, []string) app.Result

func newArgumentCommand(runtime commandRuntime, use string, short string, args cobra.PositionalArgs, operation argumentOperation) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(cmd *cobra.Command, values []string) error {
			return runtime.run(cmd, func(ctx context.Context, application app.Application) app.Result {
				return operation(ctx, application, values)
			})
		},
	}
}

func newEasyTierApplyCommand(runtime commandRuntime) *cobra.Command {
	var dryRun bool
	command := newResultCommand(runtime, "apply", "Validate, snapshot, restart and verify EasyTier", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.EasyTierApply(ctx, dryRun)
	})
	command.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示计划，不修改系统")
	return command
}

func newEasyTierRollbackCommand(runtime commandRuntime) *cobra.Command {
	var dryRun bool
	command := newArgumentCommand(runtime, "rollback SNAPSHOT_ID", "Restore a verified EasyTier snapshot", cobra.ExactArgs(1), func(ctx context.Context, application app.Application, args []string) app.Result {
		return application.EasyTierRollback(ctx, args[0], dryRun)
	})
	command.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示计划，不修改系统")
	return command
}

func newMihomoApplyCommand(runtime commandRuntime) *cobra.Command {
	var dryRun bool
	command := newResultCommand(runtime, "apply", "Validate, snapshot, restart and verify Mihomo", nil, func(ctx context.Context, application app.Application) app.Result {
		return application.MihomoApply(ctx, dryRun)
	})
	command.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示计划，不修改系统")
	return command
}

func newMihomoRollbackCommand(runtime commandRuntime) *cobra.Command {
	var dryRun bool
	command := newArgumentCommand(runtime, "rollback SNAPSHOT_ID", "Restore a verified Mihomo snapshot", cobra.ExactArgs(1), func(ctx context.Context, application app.Application, args []string) app.Result {
		return application.MihomoRollback(ctx, args[0], dryRun)
	})
	command.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示计划，不修改系统")
	return command
}

func newLogsCommand(target string, runtime commandRuntime) *cobra.Command {
	var since time.Duration
	var lines int
	var follow bool
	command := &cobra.Command{
		Use:   "logs",
		Short: "Read service logs from journald",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if follow {
				return runtime.stream(cmd, func(ctx context.Context, application app.Application, stdout io.Writer, stderr io.Writer) error {
					return application.FollowLogs(ctx, target, lines, since, stdout, stderr)
				})
			}
			return runtime.run(cmd, func(ctx context.Context, application app.Application) app.Result {
				return application.Logs(ctx, target, lines, since)
			})
		},
	}
	command.Flags().IntVarP(&lines, "tail", "n", 100, "显示末尾日志行数")
	command.Flags().DurationVar(&since, "since", 0, "仅显示此时间范围内的日志")
	command.Flags().BoolVarP(&follow, "follow", "f", false, "输出末尾日志后持续跟随新增日志")
	return command
}

// ExitCode 将 CLI 错误映射为 Netkit 文档定义的退出码。
func ExitCode(err error) int {
	if errors.Is(err, errRootRequired) {
		return 3
	}
	return 1
}
