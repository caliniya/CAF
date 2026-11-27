// Package id 生成和校验带类型前缀的 UUIDv7 标识。
package id

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// 内部对象使用的类型名；自定义类型也可直接传入 New。
const (
	User         = "user"
	Memory       = "memory"
	Recollection = "recollection"
	Chat         = "chat"
	Node         = "node"
	Config       = "config"
	Workspace    = "workspace"
	QQAdapter    = "qqadapter"
	Request      = "request"
	Archive      = "archive"
	Recovery     = "recovery"
	Attachment   = "attachment"
)

// 限制前缀为路径安全的单个类型名，保证格式中只有一个短横线。
var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// New 返回 类型-32位小写十六进制字符，例如 memory-019…。
// kind 必须以小写字母开头，仅含小写字母、数字、下划线，最长 32 位。
// UUIDv7 使用本机时间生成，无需网络；生成失败返回错误，不使用其他格式回退。
func New(kind string) (string, error) {
	if !kindPattern.MatchString(kind) {
		return "", fmt.Errorf("无效 ID 类型 %q：需以小写字母开头，仅含小写字母、数字和下划线，最长 32 位", kind)
	}
	value, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("生成 %s ID 失败：%w", kind, err)
	}
	return kind + "-" + hex.EncodeToString(value[:]), nil
}

// MustNew 用于不返回 error 的构造函数；类型无效或生成失败时 panic。
// 用户输入及可以返回错误的业务入口应使用 New。
func MustNew(kind string) string {
	value, err := New(kind)
	if err != nil {
		panic(err)
	}
	return value
}

// Valid 校验指定类型的完整格式、UUIDv7 版本和 RFC 9562 variant。
// 不接受裸 UUID、带额外短横线的 UUID、旧随机 ID 或大写十六进制格式。
func Valid(value, kind string) bool {
	if !kindPattern.MatchString(kind) {
		return false
	}
	prefix := kind + "-"
	if len(value) != len(prefix)+32 || !strings.HasPrefix(value, prefix) {
		return false
	}
	suffix := value[len(prefix):]
	for _, c := range suffix {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	parsed, err := uuid.Parse(suffix)
	return err == nil && parsed.Version() == 7 && parsed.Variant() == uuid.RFC4122
}
