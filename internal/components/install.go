// Package components 安装 Mihomo 与 EasyTier 官方二进制。
package components

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Request 描述在线或离线的完整组件安装请求。
type Request struct {
	MihomoVersion   string
	EasyTierVersion string
	MihomoPackage   string
	EasyTierPackage string
}

// Installed 描述已安装的组件。
type Installed struct {
	Name    string
	Path    string
	Version string
}

// Installer 提供可替换的路径与 HTTP 依赖，便于离线测试。
type Installer struct {
	Client    *http.Client
	APIBase   string
	BinDir    string
	BackupDir string
	GOARCH    string
}

// New 使用系统标准路径创建安装器。
func New(rootDir string) Installer {
	return Installer{
		Client:    http.DefaultClient,
		APIBase:   "https://api.github.com",
		BinDir:    "/usr/local/bin",
		BackupDir: filepath.Join(rootDir, ".netkit", "components", "backups"),
		GOARCH:    runtime.GOARCH,
	}
}

// Install 下载或解压两个组件后原子替换二进制，不重启服务。
func (i Installer) Install(ctx context.Context, request Request) ([]Installed, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	if i.GOARCH != "amd64" && i.GOARCH != "arm64" {
		return nil, fmt.Errorf("不支持的 CPU 架构 %q", i.GOARCH)
	}
	if i.Client == nil {
		i.Client = http.DefaultClient
	}
	if i.APIBase == "" {
		i.APIBase = "https://api.github.com"
	}
	dir, err := os.MkdirTemp("", "netkit-components-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	mihomo, mihomoVersion, err := i.acquire(ctx, dir, "mihomo", request.MihomoVersion, request.MihomoPackage)
	if err != nil {
		return nil, err
	}
	easyTier, easyTierVersion, err := i.acquire(ctx, dir, "easytier", request.EasyTierVersion, request.EasyTierPackage)
	if err != nil {
		return nil, err
	}
	mihomoBinary := filepath.Join(dir, "mihomo")
	if err := extractGzip(mihomo, mihomoBinary); err != nil {
		return nil, fmt.Errorf("解压 Mihomo: %w", err)
	}
	easyTierBinaries, err := extractZip(easyTier, dir)
	if err != nil {
		return nil, err
	}
	files := map[string]string{"mihomo": mihomoBinary}
	for name, path := range easyTierBinaries {
		files[name] = path
	}
	if err := i.install(files); err != nil {
		return nil, err
	}
	return []Installed{
		{Name: "mihomo", Path: filepath.Join(i.BinDir, "mihomo"), Version: mihomoVersion},
		{Name: "easytier", Path: filepath.Join(i.BinDir, "easytier-core"), Version: easyTierVersion},
	}, nil
}

func (r Request) validate() error {
	local := r.MihomoPackage != "" || r.EasyTierPackage != ""
	if !local {
		return nil
	}
	if r.MihomoPackage == "" || r.EasyTierPackage == "" {
		return fmt.Errorf("离线安装必须同时指定 --mihomo-package 与 --easytier-package")
	}
	if r.MihomoVersion != "" || r.EasyTierVersion != "" {
		return fmt.Errorf("离线包参数不能与版本参数同时使用")
	}
	return nil
}

func (i Installer) acquire(ctx context.Context, dir, name, version, local string) (string, string, error) {
	if local != "" {
		info, err := os.Stat(local)
		if err != nil {
			return "", "", fmt.Errorf("读取 %s 离线包: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("%s 离线包不是普通文件", name)
		}
		if err := validateOfflinePackage(name, i.GOARCH, filepath.Base(local)); err != nil {
			return "", "", err
		}
		return local, "local", nil
	}
	release, err := i.release(ctx, name, version)
	if err != nil {
		return "", "", err
	}
	asset, err := release.asset(name, i.GOARCH)
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, asset.Name)
	if err := i.download(ctx, asset.URL, path); err != nil {
		return "", "", err
	}
	if err := verifyDigest(path, asset.Digest); err != nil {
		return "", "", fmt.Errorf("校验 %s: %w", asset.Name, err)
	}
	return path, release.Tag, nil
}

func validateOfflinePackage(name, arch, filename string) error {
	expected := "mihomo-linux-" + arch + "-"
	extension := ".gz"
	if name == "easytier" {
		easyTierArch := "x86_64"
		if arch == "arm64" {
			easyTierArch = "aarch64"
		}
		expected = "easytier-linux-" + easyTierArch + "-"
		extension = ".zip"
	}
	if !strings.HasPrefix(filename, expected) || !strings.HasSuffix(filename, extension) {
		return fmt.Errorf("%s 离线包与当前 %s 架构不匹配: %s", name, arch, filename)
	}
	return nil
}

type release struct {
	Tag    string  `json:"tag_name"`
	Assets []asset `json:"assets"`
}

type asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}

func (i Installer) release(ctx context.Context, name, version string) (release, error) {
	repository := "MetaCubeX/mihomo"
	if name == "easytier" {
		repository = "EasyTier/EasyTier"
	}
	path := "latest"
	if version != "" {
		path = "tags/" + version
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(i.APIBase, "/")+"/repos/"+repository+"/releases/"+path, nil)
	if err != nil {
		return release{}, err
	}
	response, err := i.Client.Do(request)
	if err != nil {
		return release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("查询 %s Release: HTTP %d", name, response.StatusCode)
	}
	var value release
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return release{}, err
	}
	if value.Tag == "" {
		return release{}, fmt.Errorf("%s Release 未提供版本", name)
	}
	return value, nil
}

func (r release) asset(name, arch string) (asset, error) {
	expected := fmt.Sprintf("mihomo-linux-%s-%s.gz", arch, r.Tag)
	if name == "easytier" {
		easyTierArch := "x86_64"
		if arch == "arm64" {
			easyTierArch = "aarch64"
		}
		expected = fmt.Sprintf("easytier-linux-%s-%s.zip", easyTierArch, r.Tag)
	}
	for _, item := range r.Assets {
		if item.Name == expected {
			return item, nil
		}
	}
	return asset{}, fmt.Errorf("%s Release %s 未提供 %s", name, r.Tag, expected)
}

func (i Installer) download(ctx context.Context, url, path string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := i.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载组件: HTTP %d", response.StatusCode)
	}
	return writeFile(path, response.Body)
}

func verifyDigest(path, digest string) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return fmt.Errorf("Release 未提供 SHA-256 digest")
	}
	expected, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	if err != nil || len(expected) != sha256.Size {
		return fmt.Errorf("Release SHA-256 digest 无效")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return err
	}
	if string(sum.Sum(nil)) != string(expected) {
		return fmt.Errorf("SHA-256 不匹配")
	}
	return nil
}

func extractGzip(source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer reader.Close()
	return writeFile(destination, reader)
}

func extractZip(source, dir string) (map[string]string, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return nil, fmt.Errorf("解压 EasyTier: %w", err)
	}
	defer archive.Close()
	files := map[string]string{}
	for _, item := range archive.File {
		name := filepath.Base(item.Name)
		if name != "easytier-core" && name != "easytier-cli" {
			continue
		}
		reader, err := item.Open()
		if err != nil {
			return nil, err
		}
		err = writeFile(filepath.Join(dir, name), reader)
		reader.Close()
		if err != nil {
			return nil, err
		}
		files[name] = filepath.Join(dir, name)
	}
	if files["easytier-core"] == "" || files["easytier-cli"] == "" {
		return nil, fmt.Errorf("EasyTier 压缩包缺少 easytier-core 或 easytier-cli")
	}
	return files, nil
}

func writeFile(path string, reader io.Reader) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(path, 0o755)
}

func (i Installer) install(files map[string]string) error {
	if err := os.MkdirAll(i.BinDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(i.BackupDir, 0o700); err != nil {
		return err
	}
	backupDir := filepath.Join(i.BackupDir, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}
	replaced := []string{}
	for name, source := range files {
		target := filepath.Join(i.BinDir, name)
		if _, err := os.Stat(target); err == nil {
			if err := copyFile(target, filepath.Join(backupDir, name)); err != nil {
				return err
			}
		}
		if err := atomicCopy(source, target); err != nil {
			i.restore(replaced, backupDir)
			return fmt.Errorf("安装 %s: %w", name, err)
		}
		replaced = append(replaced, name)
	}
	return nil
}

func (i Installer) restore(names []string, backupDir string) {
	for _, name := range names {
		backup := filepath.Join(backupDir, name)
		target := filepath.Join(i.BinDir, name)
		if _, err := os.Stat(backup); err == nil {
			_ = atomicCopy(backup, target)
			continue
		}
		_ = os.Remove(target)
	}
}

func copyFile(source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	return writeFile(destination, file)
}

func atomicCopy(source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".netkit-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, file); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o755); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}
