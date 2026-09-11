// Package config 负责解析 Netkit 的文件路径和可执行文件位置。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultRoot = "/root/netkit"

const (
	ServiceMihomo   = "mihomo"
	ServiceEasyTier = "easytier"
)

// ManagedService 描述一个由 Netkit 管理的网络服务及其权威资产。
type ManagedService struct {
	Name        string
	Unit        string
	ConfigPath  string
	SourceFiles []string
}

// Config 表示已解析的运行时配置。
type Config struct {
	RootDir         string
	MihomoBinary    string
	EasyTierCLI     string
	MihomoProxyURL  string
	MihomoHealthURL string
	Timeout         time.Duration
	Mihomo          ManagedService
	EasyTier        ManagedService
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
		EasyTierCLI:     "/usr/local/bin/easytier-cli",
		MihomoProxyURL:  "http://127.0.0.1:7890",
		MihomoHealthURL: "https://www.gstatic.com/generate_204",
		Timeout:         timeout,
		Mihomo: ManagedService{
			Name:        ServiceMihomo,
			Unit:        "mihomo.service",
			ConfigPath:  "/etc/mihomo",
			SourceFiles: []string{"mihomo/config/config.yaml", "services/mihomo.service"},
		},
		EasyTier: ManagedService{
			Name:        ServiceEasyTier,
			Unit:        "easytier.service",
			ConfigPath:  "/etc/easytier/config.toml",
			SourceFiles: []string{"easytier/config/config.toml", "services/easytier.service"},
		},
	}, nil
}

// SnapshotDir 返回 Netkit 管理的快照目录。
func (c Config) SnapshotDir() string { return filepath.Join(c.RootDir, ".netkit", "snapshots") }

// AuditLog 返回 Netkit 追加写入的审计日志路径。
func (c Config) AuditLog() string { return filepath.Join(c.RootDir, ".netkit", "audit.jsonl") }

// MihomoSourceDir 返回 Mihomo 配置的权威目录。
func (c Config) MihomoSourceDir() string {
	return filepath.Dir(filepath.Join(c.RootDir, c.Mihomo.SourceFiles[0]))
}

// EasyTierSourceFile 返回 EasyTier 配置的权威文件。
func (c Config) EasyTierSourceFile() string {
	return filepath.Join(c.RootDir, c.EasyTier.SourceFiles[0])
}

// Service 返回指定受管服务的定义副本。
func (c Config) Service(name string) (ManagedService, error) {
	switch name {
	case ServiceMihomo:
		return cloneService(c.Mihomo), nil
	case ServiceEasyTier:
		return cloneService(c.EasyTier), nil
	default:
		return ManagedService{}, fmt.Errorf("unknown service %q", name)
	}
}

// ServicesForTarget 返回快照目标包含的受管服务定义。
func (c Config) ServicesForTarget(target string) ([]ManagedService, error) {
	switch target {
	case ServiceMihomo, ServiceEasyTier:
		service, err := c.Service(target)
		if err != nil {
			return nil, err
		}
		return []ManagedService{service}, nil
	case "all":
		return []ManagedService{cloneService(c.Mihomo), cloneService(c.EasyTier)}, nil
	default:
		return nil, fmt.Errorf("不支持的快照目标 %q", target)
	}
}

// ServiceFile 返回指定服务的权威 systemd unit 文件路径。
func (c Config) ServiceFile(name string) (string, error) {
	service, err := c.Service(name)
	if err != nil {
		return "", err
	}
	for _, sourceFile := range service.SourceFiles {
		if filepath.Ext(sourceFile) == ".service" {
			return filepath.Join(c.RootDir, sourceFile), nil
		}
	}
	return "", fmt.Errorf("服务 %q 未定义 unit 文件", name)
}

func cloneService(service ManagedService) ManagedService {
	service.SourceFiles = append([]string{}, service.SourceFiles...)
	return service
}
