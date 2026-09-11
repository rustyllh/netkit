// Package snapshot 管理网络配置的受限快照和完整性校验。
package snapshot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rustyllh/netkit/internal/config"
)

// File 表示快照中一个文件的完整性元数据。
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest 描述一个可验证的配置快照。
type Manifest struct {
	ID        string    `json:"id"`
	Target    string    `json:"target"`
	CreatedAt time.Time `json:"created_at"`
	Files     []File    `json:"files"`
}

// Store 管理指定 Netkit 配置下的快照。
type Store struct{ config config.Config }

// New 创建快照存储。
func New(cfg config.Config) Store { return Store{config: cfg} }

// Create 为 target 创建快照。target 可为 mihomo、easytier 或 all。
func (s Store) Create(ctx context.Context, target string) (Manifest, error) {
	files, err := s.sourceFiles(target)
	if err != nil {
		return Manifest{}, err
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	base := filepath.Join(s.config.RootDir, ".netkit", "snapshots")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("创建快照目录: %w", err)
	}
	if err := os.Chmod(base, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("设置快照目录权限: %w", err)
	}
	id, err := snapshotID(target)
	if err != nil {
		return Manifest{}, err
	}
	temporary, err := os.MkdirTemp(base, ".tmp-")
	if err != nil {
		return Manifest{}, fmt.Errorf("创建临时快照: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := os.Chmod(temporary, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("设置临时快照权限: %w", err)
	}

	manifest := Manifest{ID: id, Target: target, CreatedAt: time.Now().UTC(), Files: make([]File, 0, len(files))}
	for _, relative := range files {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		source, err := s.safeSource(relative)
		if err != nil {
			return Manifest{}, err
		}
		destination := filepath.Join(temporary, "files", relative)
		digest, size, err := copyFile(source, destination)
		if err != nil {
			return Manifest{}, fmt.Errorf("快照 %s: %w", relative, err)
		}
		manifest.Files = append(manifest.Files, File{Path: relative, SHA256: digest, Size: size})
	}
	if err := writeManifest(temporary, manifest); err != nil {
		return Manifest{}, err
	}
	if err := os.Rename(temporary, filepath.Join(base, id)); err != nil {
		return Manifest{}, fmt.Errorf("提交快照: %w", err)
	}
	return manifest, nil
}

// List 返回所有已提交快照，按创建时间从新到旧排列。
func (s Store) List(ctx context.Context) ([]Manifest, error) {
	entries, err := os.ReadDir(filepath.Join(s.config.RootDir, ".netkit", "snapshots"))
	if os.IsNotExist(err) {
		return []Manifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取快照目录: %w", err)
	}
	manifests := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		manifest, err := s.readManifest(entry.Name())
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].CreatedAt.After(manifests[j].CreatedAt) })
	return manifests, nil
}

// Verify 校验快照清单、文件路径、大小和 SHA-256。
func (s Store) Verify(ctx context.Context, id string) (Manifest, error) {
	manifest, err := s.readManifest(id)
	if err != nil {
		return Manifest{}, err
	}
	for _, file := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		if !validRelativePath(file.Path) {
			return Manifest{}, fmt.Errorf("快照包含非法路径 %q", file.Path)
		}
		path := filepath.Join(s.config.RootDir, ".netkit", "snapshots", id, "files", file.Path)
		digest, size, err := checksum(path)
		if err != nil {
			return Manifest{}, fmt.Errorf("校验 %s: %w", file.Path, err)
		}
		if size != file.Size || digest != file.SHA256 {
			return Manifest{}, fmt.Errorf("快照文件校验失败: %s", file.Path)
		}
	}
	return manifest, nil
}

// Restore 将已验证快照的文件原子恢复到 Netkit 权威目录。
func (s Store) Restore(ctx context.Context, id, target string) (Manifest, error) {
	manifest, err := s.Verify(ctx, id)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.Target != target {
		return Manifest{}, fmt.Errorf("快照目标 %q 与恢复目标 %q 不一致", manifest.Target, target)
	}
	for _, file := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		source := filepath.Join(s.config.RootDir, ".netkit", "snapshots", id, "files", file.Path)
		destination, err := s.safeSourceDestination(file.Path)
		if err != nil {
			return Manifest{}, err
		}
		if err := restoreFile(source, destination); err != nil {
			return Manifest{}, fmt.Errorf("恢复 %s: %w", file.Path, err)
		}
	}
	return manifest, nil
}

func (s Store) sourceFiles(target string) ([]string, error) {
	services, err := s.config.ServicesForTarget(target)
	if err != nil {
		return nil, err
	}
	files := []string{}
	for _, service := range services {
		files = append(files, service.SourceFiles...)
	}
	return files, nil
}

func (s Store) safeSource(relative string) (string, error) {
	if !validRelativePath(relative) {
		return "", fmt.Errorf("非法源路径 %q", relative)
	}
	path := filepath.Join(s.config.RootDir, relative)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("不是普通文件: %s", relative)
	}
	return path, nil
}

func (s Store) safeSourceDestination(relative string) (string, error) {
	if !validRelativePath(relative) {
		return "", fmt.Errorf("非法目标路径 %q", relative)
	}
	return filepath.Join(s.config.RootDir, relative), nil
}

func (s Store) readManifest(id string) (Manifest, error) {
	if !validID(id) {
		return Manifest{}, fmt.Errorf("非法快照 ID %q", id)
	}
	data, err := os.ReadFile(filepath.Join(s.config.RootDir, ".netkit", "snapshots", id, "manifest.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("读取快照清单: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("解析快照清单: %w", err)
	}
	if manifest.ID != id || len(manifest.Files) == 0 {
		return Manifest{}, fmt.Errorf("无效快照清单 %q", id)
	}
	return manifest, nil
}

func writeManifest(directory string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化快照清单: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0o600); err != nil {
		return fmt.Errorf("写入快照清单: %w", err)
	}
	var lines strings.Builder
	for _, file := range manifest.Files {
		fmt.Fprintf(&lines, "%s  files/%s\n", file.SHA256, file.Path)
	}
	if err := os.WriteFile(filepath.Join(directory, "checksums.sha256"), []byte(lines.String()), 0o600); err != nil {
		return fmt.Errorf("写入校验清单: %w", err)
	}
	return nil
}

func copyFile(source, destination string) (string, int64, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", 0, err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", 0, err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return "", 0, copyErr
	}
	if syncErr != nil {
		return "", 0, syncErr
	}
	if closeErr != nil {
		return "", 0, closeErr
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func restoreFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".netkit-restore-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, input); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, destination)
}

func checksum(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func snapshotID(target string) (string, error) {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("生成快照 ID: %w", err)
	}
	return fmt.Sprintf("%s-%s-%s", time.Now().UTC().Format("20060102T150405Z"), target, hex.EncodeToString(bytes)), nil
}

func validID(id string) bool {
	return !strings.Contains(id, "/") && !strings.Contains(id, "\\") && !strings.Contains(id, "..") && id != ""
}
func validRelativePath(path string) bool {
	return !filepath.IsAbs(path) && path == filepath.Clean(path) && !strings.HasPrefix(path, "..")
}
