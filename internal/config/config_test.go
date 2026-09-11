package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name, root string
		wantRoot   string
		wantErr    bool
	}{
		{"default", "", "/root/netkit", false},
		{"absolute root", "/tmp/netkit", "/tmp/netkit", false},
		{"relative root rejected", "netkit", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := Load(test.root, time.Second)
			if (err != nil) != test.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && cfg.RootDir != test.wantRoot {
				t.Fatalf("RootDir = %q, want %q", cfg.RootDir, test.wantRoot)
			}
		})
	}
}

func TestServicesForTarget(t *testing.T) {
	cfg, err := Load("/tmp/netkit", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		target    string
		wantNames []string
		wantFiles int
	}{
		{
			name:      "单个 Mihomo 服务",
			target:    ServiceMihomo,
			wantNames: []string{ServiceMihomo},
			wantFiles: 2,
		},
		{
			name:      "全部服务",
			target:    "all",
			wantNames: []string{ServiceMihomo, ServiceEasyTier},
			wantFiles: 4,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			services, err := cfg.ServicesForTarget(test.target)
			if err != nil {
				t.Fatal(err)
			}
			if len(services) != len(test.wantNames) {
				t.Fatalf("服务数 = %d，期望 %d", len(services), len(test.wantNames))
			}
			files := 0
			for index, service := range services {
				if service.Name != test.wantNames[index] {
					t.Fatalf("服务[%d] = %q，期望 %q", index, service.Name, test.wantNames[index])
				}
				files += len(service.SourceFiles)
			}
			if files != test.wantFiles {
				t.Fatalf("快照文件数 = %d，期望 %d", files, test.wantFiles)
			}
		})
	}
}

func TestServiceReturnsIndependentSourceFiles(t *testing.T) {
	cfg, err := Load("/tmp/netkit", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	service, err := cfg.Service(ServiceMihomo)
	if err != nil {
		t.Fatal(err)
	}
	service.SourceFiles[0] = "changed"
	if cfg.Mihomo.SourceFiles[0] == "changed" {
		t.Fatal("Service() 不应暴露配置中的可变切片")
	}
}
