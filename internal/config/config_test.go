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
