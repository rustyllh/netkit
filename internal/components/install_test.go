package components

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		wantErr bool
	}{
		{name: "在线 latest"},
		{name: "在线指定版本", request: Request{MihomoVersion: "v1.0.0"}},
		{name: "完整离线包", request: Request{MihomoPackage: "mihomo.gz", EasyTierPackage: "easytier.zip"}},
		{name: "离线包不完整", request: Request{MihomoPackage: "mihomo.gz"}, wantErr: true},
		{name: "混合来源", request: Request{MihomoPackage: "mihomo.gz", EasyTierPackage: "easytier.zip", MihomoVersion: "v1.0.0"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.request.validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestInstallerInstallLocalPackages(t *testing.T) {
	dir := t.TempDir()
	mihomoPackage := filepath.Join(dir, "mihomo-linux-amd64-v1.0.0.gz")
	easyTierPackage := filepath.Join(dir, "easytier-linux-x86_64-v1.0.0.zip")
	writeGzip(t, mihomoPackage, "mihomo binary")
	writeZip(t, easyTierPackage, map[string]string{
		"easytier-core": "core binary",
		"easytier-cli":  "cli binary",
	})
	installer := Installer{
		BinDir:    filepath.Join(dir, "bin"),
		BackupDir: filepath.Join(dir, "backups"),
		GOARCH:    "amd64",
	}
	installed, err := installer.Install(context.Background(), Request{
		MihomoPackage:   mihomoPackage,
		EasyTierPackage: easyTierPackage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 2 {
		t.Fatalf("安装结果数 = %d, 期望 2", len(installed))
	}
	for _, name := range []string{"mihomo", "easytier-core", "easytier-cli"} {
		data, err := os.ReadFile(filepath.Join(installer.BinDir, name))
		if err != nil {
			t.Fatalf("读取 %s: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s 未写入内容", name)
		}
	}
}

func TestReleaseAsset(t *testing.T) {
	release := release{Tag: "v1.2.3", Assets: []asset{
		{Name: "mihomo-linux-amd64-v1.2.3.gz"},
		{Name: "easytier-linux-aarch64-v1.2.3.zip"},
	}}
	if _, err := release.asset("mihomo", "amd64"); err != nil {
		t.Fatalf("选择 Mihomo 资产: %v", err)
	}
	if _, err := release.asset("easytier", "arm64"); err != nil {
		t.Fatalf("选择 EasyTier 资产: %v", err)
	}
	if _, err := release.asset("mihomo", "arm64"); err == nil {
		t.Fatal("缺少资产时应失败")
	}
}

func TestInstallerAcquireOnline(t *testing.T) {
	body := []byte("release package")
	digest := sha256.Sum256(body)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/asset" {
			_, _ = writer.Write(body)
			return
		}
		value := release{
			Tag: "v1.2.3",
			Assets: []asset{{
				Name:   "mihomo-linux-amd64-v1.2.3.gz",
				URL:    server.URL + "/asset",
				Digest: "sha256:" + hex.EncodeToString(digest[:]),
			}},
		}
		if err := json.NewEncoder(writer).Encode(value); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	installer := Installer{Client: server.Client(), APIBase: server.URL, GOARCH: "amd64"}
	path, version, err := installer.acquire(context.Background(), t.TempDir(), "mihomo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1.2.3" {
		t.Fatalf("版本 = %q", version)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(body) {
		t.Fatalf("下载内容 = %q", data)
	}
}

func TestValidateOfflinePackage(t *testing.T) {
	if err := validateOfflinePackage("mihomo", "amd64", "mihomo-linux-amd64-v1.0.0.gz"); err != nil {
		t.Fatal(err)
	}
	if err := validateOfflinePackage("easytier", "amd64", "easytier-linux-aarch64-v1.0.0.zip"); err == nil {
		t.Fatal("错误架构应失败")
	}
}

func writeGzip(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
