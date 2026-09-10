// Package audit 负责安全追加 Netkit 的脱敏审计事件。
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var sensitive = regexp.MustCompile(`(?i)(token|secret|authorization|password|api[_-]?key)=?[^\s,;&]+`)

// Event 是一条审计事件。
type Event struct {
	Time      time.Time         `json:"time"`
	UID       int               `json:"uid"`
	Operation string            `json:"operation"`
	Target    string            `json:"target"`
	Snapshot  string            `json:"snapshot,omitempty"`
	Result    string            `json:"result"`
	Details   map[string]string `json:"details,omitempty"`
}

// Append 脱敏后以单次追加写入的方式记录审计事件。
func Append(ctx context.Context, root string, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	directory := filepath.Join(root, ".netkit")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("创建审计目录: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("设置审计目录权限: %w", err)
	}
	event.Time = time.Now().UTC()
	event.UID = os.Geteuid()
	event.Details = redact(event.Details)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化审计事件: %w", err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(filepath.Join(directory, "audit.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("打开审计日志: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("追加审计日志: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步审计日志: %w", err)
	}
	return nil
}

func redact(details map[string]string) map[string]string {
	result := make(map[string]string, len(details))
	for key, value := range details {
		result[key] = sensitive.ReplaceAllString(value, "$1=***")
	}
	return result
}
