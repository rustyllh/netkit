package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rustyllh/netkit/internal/config"
)

func TestCreateAndVerify(t *testing.T) {
	root := testRoot(t)
	store := newStore(t, root)
	manifest, err := store.Create(context.Background(), "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 4 {
		t.Fatalf("文件数 = %d, want 4", len(manifest.Files))
	}
	if _, err := os.Stat(filepath.Join(root, ".netkit", "snapshots", manifest.ID, "files", "mihomo", "config", "proxy_providers")); !os.IsNotExist(err) {
		t.Fatalf("provider 缓存不应被快照，err=%v", err)
	}
	if _, err := store.Verify(context.Background(), manifest.ID); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(root, ".netkit", "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("目录权限 = %o, want 700", info.Mode().Perm())
	}
}

func TestVerifyRejectsTamperingAndTraversal(t *testing.T) {
	root := testRoot(t)
	store := newStore(t, root)
	manifest, err := store.Create(context.Background(), "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".netkit", "snapshots", manifest.ID, "files", "mihomo", "config", "config.yaml")
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Verify(context.Background(), manifest.ID); err == nil {
		t.Fatal("篡改后 Verify() 应失败")
	}
	if _, err := store.Verify(context.Background(), "../outside"); err == nil {
		t.Fatal("路径穿越 ID 应失败")
	}
}

func TestCreateFailsWhenSnapshotLocationIsBlocked(t *testing.T) {
	root := testRoot(t)
	if err := os.WriteFile(filepath.Join(root, ".netkit"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newStore(t, root).Create(context.Background(), "mihomo"); err == nil {
		t.Fatal("快照位置不可创建时 Create() 应失败")
	}
}

func TestRestore(t *testing.T) {
	root := testRoot(t)
	store := newStore(t, root)
	manifest, err := store.Create(context.Background(), "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "mihomo", "config", "config.yaml")
	if err := os.WriteFile(config, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore(context.Background(), manifest.ID, "mihomo"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mixed-port: 7890\n" {
		t.Fatalf("恢复结果 = %q", data)
	}
}

func testRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"mihomo/config/config.yaml":                "mixed-port: 7890\n",
		"easytier/config/config.toml":              "instance_name = 'test'\n",
		"services/mihomo.service":                  "[Service]\n",
		"services/easytier.service":                "[Service]\n",
		"mihomo/config/proxy_providers/cache.yaml": "not-a-source-config\n",
	}
	for relative, content := range files {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func newStore(t *testing.T, root string) Store {
	t.Helper()
	cfg, err := config.Load(root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg)
}
