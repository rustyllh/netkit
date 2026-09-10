// Package cli 定义 Netkit 的命令行接口。
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rustyllh/netkit/internal/app"
	"github.com/rustyllh/netkit/internal/config"
	"github.com/rustyllh/netkit/internal/linux"
	"github.com/spf13/cobra"
)

var errRootRequired = errors.New("netkit must run as root")

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
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if os.Geteuid() != 0 {
				return errRootRequired
			}
			return nil
		},
	}
	command.PersistentFlags().StringVar(&rootDir, "root", "", "Netkit asset root (default /root/netkit)")
	command.PersistentFlags().DurationVar(&timeout, "timeout", 30*time.Second, "external command timeout")
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit JSON results")

	newApp := func() (app.Application, error) {
		cfg, err := config.Load(rootDir, timeout)
		if err != nil {
			return app.Application{}, err
		}
		return app.New(cfg, linux.OSRunner{}), nil
	}
	render := func(cmd *cobra.Command, value app.Result) error {
		return writeResult(cmd.OutOrStdout(), value, jsonOutput)
	}

	command.AddCommand(&cobra.Command{Use: "status", Short: "Show managed service status", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.Status(ctx))
	}})

	links := &cobra.Command{Use: "links", Short: "Inspect standard Linux entry-point links"}
	links.AddCommand(&cobra.Command{Use: "verify", Short: "Verify links resolve into the Netkit root", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		return render(cmd, application.VerifyLinks(cmd.Context()))
	}})
	command.AddCommand(links)

	mihomo := &cobra.Command{Use: "mihomo", Short: "Operate on Mihomo"}
	mihomo.AddCommand(&cobra.Command{Use: "validate", Short: "Validate active Mihomo configuration", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.ValidateMihomo(ctx))
	}})
	mihomo.AddCommand(newLogsCommand("mihomo", newApp, render, &timeout))
	mihomo.AddCommand(newMihomoRestartCommand(newApp, render, &timeout))
	mihomo.AddCommand(newMihomoApplyCommand(newApp, render, &timeout))
	mihomo.AddCommand(newMihomoRollbackCommand(newApp, render, &timeout))
	command.AddCommand(mihomo)

	easytier := &cobra.Command{Use: "easytier", Short: "Operate on EasyTier"}
	easytier.AddCommand(&cobra.Command{Use: "status", Short: "Show EasyTier status", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.EasyTierStatus(ctx))
	}})
	easytier.AddCommand(&cobra.Command{Use: "validate", Short: "Validate EasyTier TOML configuration", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.ValidateEasyTier(ctx))
	}})
	easytier.AddCommand(&cobra.Command{Use: "peers", Short: "Show EasyTier peers", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.EasyTierPeers(ctx))
	}})
	easytier.AddCommand(&cobra.Command{Use: "ping IP", Short: "Ping an EasyTier peer", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.PingEasyTierPeer(ctx, args[0]))
	}})
	easytier.AddCommand(newLogsCommand("easytier", newApp, render, &timeout))
	easytier.AddCommand(newEasyTierRestartCommand(newApp, render, &timeout))
	easytier.AddCommand(newEasyTierApplyCommand(newApp, render, &timeout))
	easytier.AddCommand(newEasyTierRollbackCommand(newApp, render, &timeout))
	command.AddCommand(easytier)

	backup := &cobra.Command{Use: "backup", Short: "Create and verify configuration snapshots"}
	backup.AddCommand(&cobra.Command{Use: "create [mihomo|easytier|all]", Short: "Create a configuration snapshot", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		target := "all"
		if len(args) == 1 {
			target = args[0]
		}
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.CreateBackup(ctx, target))
	}})
	backup.AddCommand(&cobra.Command{Use: "list", Short: "List configuration snapshots", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.ListBackups(ctx))
	}})
	backup.AddCommand(&cobra.Command{Use: "verify SNAPSHOT_ID", Short: "Verify a configuration snapshot", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.VerifyBackup(ctx, args[0]))
	}})
	command.AddCommand(backup)

	command.AddCommand(&cobra.Command{Use: "doctor", Short: "Run host and network diagnostics", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		return render(cmd, application.Doctor(ctx))
	}})
	return command
}

func newEasyTierRestartCommand(newApp func() (app.Application, error), render func(*cobra.Command, app.Result) error, timeout *time.Duration) *cobra.Command {
	command := &cobra.Command{Use: "restart", Short: "Restart EasyTier", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), *timeout)
		defer cancel()
		return render(cmd, application.EasyTierRestart(ctx))
	}}
	return command
}

func newEasyTierApplyCommand(newApp func() (app.Application, error), render func(*cobra.Command, app.Result) error, timeout *time.Duration) *cobra.Command {
	var dryRun bool
	command := &cobra.Command{Use: "apply", Short: "Validate, snapshot, restart and verify EasyTier", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), *timeout)
		defer cancel()
		return render(cmd, application.EasyTierApply(ctx, dryRun))
	}}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示计划，不修改系统")
	return command
}

func newEasyTierRollbackCommand(newApp func() (app.Application, error), render func(*cobra.Command, app.Result) error, timeout *time.Duration) *cobra.Command {
	var dryRun bool
	command := &cobra.Command{Use: "rollback SNAPSHOT_ID", Short: "Restore a verified EasyTier snapshot", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), *timeout)
		defer cancel()
		return render(cmd, application.EasyTierRollback(ctx, args[0], dryRun))
	}}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示计划，不修改系统")
	return command
}

func newMihomoRestartCommand(newApp func() (app.Application, error), render func(*cobra.Command, app.Result) error, timeout *time.Duration) *cobra.Command {
	command := &cobra.Command{Use: "restart", Short: "Restart Mihomo", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), *timeout)
		defer cancel()
		return render(cmd, application.MihomoRestart(ctx))
	}}
	return command
}

func newMihomoApplyCommand(newApp func() (app.Application, error), render func(*cobra.Command, app.Result) error, timeout *time.Duration) *cobra.Command {
	var dryRun bool
	command := &cobra.Command{Use: "apply", Short: "Validate, snapshot, restart and verify Mihomo", RunE: func(cmd *cobra.Command, _ []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), *timeout)
		defer cancel()
		return render(cmd, application.MihomoApply(ctx, dryRun))
	}}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示计划，不修改系统")
	return command
}

func newMihomoRollbackCommand(newApp func() (app.Application, error), render func(*cobra.Command, app.Result) error, timeout *time.Duration) *cobra.Command {
	var dryRun bool
	command := &cobra.Command{Use: "rollback SNAPSHOT_ID", Short: "Restore a verified Mihomo snapshot", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		application, err := newApp()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), *timeout)
		defer cancel()
		return render(cmd, application.MihomoRollback(ctx, args[0], dryRun))
	}}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "仅展示计划，不修改系统")
	return command
}

func newLogsCommand(target string, newApp func() (app.Application, error), render func(*cobra.Command, app.Result) error, timeout *time.Duration) *cobra.Command {
	var since time.Duration
	var follow bool
	command := &cobra.Command{
		Use:   "logs",
		Short: "Read service logs from journald",
		RunE: func(cmd *cobra.Command, _ []string) error {
			application, err := newApp()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), *timeout)
			defer cancel()
			return render(cmd, application.Logs(ctx, target, since, follow))
		},
	}
	command.Flags().DurationVar(&since, "since", time.Hour, "读取多久以前开始的日志")
	command.Flags().BoolVarP(&follow, "follow", "f", false, "持续输出新增日志")
	return command
}

func writeResult(writer io.Writer, result app.Result, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(writer).Encode(result)
	}
	for _, check := range result.Checks {
		state := "ok"
		if !check.OK {
			state = check.Severity
		}
		if _, err := fmt.Fprintf(writer, "%-7s %-24s %s\n", state, check.Name, check.Detail); err != nil {
			return err
		}
	}
	if !result.OK {
		return errors.New("one or more checks failed")
	}
	return nil
}

// ExitCode 将 CLI 错误映射为 Netkit 文档定义的退出码。
func ExitCode(err error) int {
	if errors.Is(err, errRootRequired) {
		return 3
	}
	return 1
}
