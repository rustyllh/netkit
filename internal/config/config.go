// Package config 负责解析 Netkit 的文件路径和可执行文件位置。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultRoot = "/root/netkit"

// Config 表示已解析的运行时配置。
type Config struct {
	RootDir         string
	MihomoBinary    string
	MihomoUnit      string
	MihomoConfig    string
	EasyTierCLI     string
	EasyTierUnit    string
	EasyTierConfig  string
	MihomoProxyURL  string
	MihomoHealthURL string
	Timeout         time.Duration
}

// Load 按 rootDir、NETKIT_ROOT、生产默认值的顺序加载配置。
func Load(rootDir string, timeout time.Duration) (Config, error) {
	if rootDir == "" {
		rootDir = os.Getenv("NETKIT_ROOT")
	}
	if rootDir == "" {
		rootDir = defaultRoot
	}
	if !filepath.IsAbs(rootDir) {
		return Config{}, fmt.Errorf("netkit root must be an absolute path: %q", rootDir)
	}
	if timeout <= 0 {
		return Config{}, fmt.Errorf("timeout must be positive")
	}

	return Config{
		RootDir:         filepath.Clean(rootDir),
		MihomoBinary:    "/usr/local/bin/mihomo",
		MihomoUnit:      "mihomo.service",
		MihomoConfig:    "/etc/mihomo",
		EasyTierCLI:     "/usr/local/bin/easytier-cli",
		EasyTierUnit:    "easytier.service",
		EasyTierConfig:  "/etc/easytier/config.toml",
		MihomoProxyURL:  "http://127.0.0.1:7890",
		MihomoHealthURL: "https://www.gstatic.com/generate_204",
		Timeout:         timeout,
	}, nil
}

// SnapshotDir 返回 Netkit 管理的快照目录。
func (c Config) SnapshotDir() string { return filepath.Join(c.RootDir, ".netkit", "snapshots") }

// AuditLog 返回 Netkit 追加写入的审计日志路径。
func (c Config) AuditLog() string { return filepath.Join(c.RootDir, ".netkit", "audit.jsonl") }

// MihomoSourceDir 返回 Mihomo 配置的权威目录。
func (c Config) MihomoSourceDir() string { return filepath.Join(c.RootDir, "mihomo", "config") }

// EasyTierSourceFile 返回 EasyTier 配置的权威文件。
func (c Config) EasyTierSourceFile() string {
	return filepath.Join(c.RootDir, "easytier", "config", "config.toml")
}

// ServiceFile 返回指定服务的权威 systemd unit 文件路径。
func (c Config) ServiceFile(name string) (string, error) {
	switch name {
	case "mihomo":
		return filepath.Join(c.RootDir, "services", "mihomo.service"), nil
	case "easytier":
		return filepath.Join(c.RootDir, "services", "easytier.service"), nil
	default:
		return "", fmt.Errorf("unknown service %q", name)
	}
}
