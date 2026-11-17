package Memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/caliniya/caf/utils/id"
)

func validateName(name string) error {
	if name == "" || strings.TrimSpace(name) != name || strings.HasPrefix(name, ".") ||
		strings.HasSuffix(name, ".") || strings.ContainsAny(name, `<>:"/\|?*`) || utf8.RuneCountInString(name) > 128 {
		return errors.New("记忆库名称必须是有效目录名，不能含路径字符、首尾空白或末尾点")
	}
	for _, c := range name {
		if c < 32 {
			return errors.New("记忆库名称不能包含控制字符")
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	device := []rune(base)
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
		(len(device) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && strings.ContainsRune("123456789¹²³", device[3])) {
		return errors.New("记忆库名称不能使用系统保留名称")
	}
	return nil
}

func validateMemory(memoryID string, memory Memory) error {
	if !id.Valid(memoryID, id.Memory) {
		return errors.New("无效记忆单元 ID")
	}
	_, err := time.Parse(time.RFC3339Nano, memory.MemoryDate)
	return err
}

func validateData(meta RecollectionMeta, r Recollection) error {
	if r.Name != meta.Name || r.Description != meta.Description || r.Data == nil {
		return fmt.Errorf("记忆库 %q 的 data.json 与元数据不一致", meta.Name)
	}
	for key, memory := range r.Data {
		if err := validateMemory(key, memory); err != nil {
			return fmt.Errorf("记忆库 %q 中的记忆损坏：%w", meta.Name, err)
		}
	}
	return nil
}

func (m *MemoryServer) readJSON(path string, target any) error {
	data, err := m.fs.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("解析 %s 失败：%w", filepath.Base(path), err)
	}
	return nil
}

// 索引存在时严格对照，不覆盖损坏数据；索引缺失时从各库 meta 重建。
func (m *MemoryServer) loadLocked() error { return m.loadIndexLocked(false) }

func (m *MemoryServer) loadIndexLocked(rebuild bool) error {
	entries, err := m.fs.ReadDir(m.root)
	if err != nil {
		return err
	}
	loaded, index := map[string]Recollection{}, map[string]string{}
	names := map[string]bool{}
	for _, entry := range entries {
		if entry.Name() == ".archive" || !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if err := validateName(name); err != nil {
			return err
		}
		var meta RecollectionMeta
		if err := m.readJSON(filepath.Join(m.root, name, "meta.json"), &meta); err != nil {
			return fmt.Errorf("读取记忆库 %q：%w", name, err)
		}
		if err := validateMeta(meta); err != nil {
			return err
		}
		if meta.Name != name || names[strings.ToLower(name)] || loaded[meta.ID].Data != nil {
			return errors.New("记忆库目录、名称或 ID 冲突")
		}
		var r Recollection
		if err := m.readJSON(filepath.Join(m.root, name, "data.json"), &r); err != nil {
			return err
		}
		if err := validateData(meta, r); err != nil {
			return err
		}
		// 历史内容只在修改旧记忆或主动查询历史时解析。
		if info, err := m.fs.Stat(filepath.Join(m.root, name, "history.json")); err != nil {
			return err
		} else if info.IsDir() {
			return errors.New("history.json 不是文件")
		}
		loaded[meta.ID], index[meta.ID], names[strings.ToLower(name)] = r, name, true
	}
	var stored map[string]string
	err = m.readJSON(filepath.Join(m.root, "index.json"), &stored)
	if rebuild || errors.Is(err, fs.ErrNotExist) {
		if err := m.writeJSON(filepath.Join(m.root, "index.json"), index); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !reflect.DeepEqual(stored, index) {
		return errors.New("记忆库索引与目录元数据不一致")
	}
	m.Recollections = loaded
	return nil
}

// 正常完成后删除的写入记录，不属于记忆业务格式，不增加版本或时间字段。
type pendingWrite struct {
	From  string                     `json:"from,omitempty"`
	To    string                     `json:"to,omitempty"`
	Files map[string]json.RawMessage `json:"files"`
}

func (m *MemoryServer) commitLocked(from, to string, files map[string]any) error {
	pending := pendingWrite{From: from, To: to, Files: make(map[string]json.RawMessage, len(files))}
	for path, value := range files {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		pending.Files[path] = data
	}
	if err := m.writeJSON(filepath.Join(m.root, ".pending.json"), pending); err != nil {
		return err
	}
	if _, err := m.recoverLocked(); err != nil {
		return fmt.Errorf("提交记忆失败，下次读取或初始化会重试恢复：%w", err)
	}
	return nil
}

func validWritePath(path string) bool {
	if path == "index.json" {
		return true
	}
	parts := strings.Split(path, "/")
	return len(parts) == 2 && validateName(parts[0]) == nil &&
		(parts[1] == "meta.json" || parts[1] == "data.json" || parts[1] == "history.json")
}

func (m *MemoryServer) recoverLocked() (bool, error) {
	var pending pendingWrite
	if err := m.readJSON(filepath.Join(m.root, ".pending.json"), &pending); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if len(pending.Files) == 0 {
		return false, errors.New("记忆提交记录无效")
	}
	for path, data := range pending.Files {
		if !validWritePath(path) || !json.Valid(data) {
			return false, errors.New("记忆提交包含无效文件路径或内容")
		}
	}
	if pending.From != "" || pending.To != "" {
		archive := strings.TrimPrefix(pending.To, ".archive/")
		if validateName(pending.From) != nil || (validateName(pending.To) != nil &&
			(!strings.HasPrefix(pending.To, ".archive/") || !id.Valid(archive, id.Recollection))) {
			return false, errors.New("记忆目录移动记录无效")
		}
		from, to := filepath.Join(m.root, pending.From), filepath.Join(m.root, pending.To)
		_, fromErr := m.fs.Stat(from)
		_, toErr := m.fs.Stat(to)
		if fromErr == nil {
			if !errors.Is(toErr, fs.ErrNotExist) {
				return false, errors.New("目标记忆目录已存在或无法读取")
			}
			if err := m.fs.CreateDirectory(filepath.Dir(to), 0o700); err != nil {
				return false, err
			}
			if err := m.fs.MoveFile(from, to); err != nil {
				return false, err
			}
		} else if !errors.Is(fromErr, fs.ErrNotExist) || toErr != nil {
			return false, errors.New("无法恢复记忆目录移动")
		}
	}
	paths := make([]string, 0, len(pending.Files))
	for path := range pending.Files {
		if path != "index.json" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	if _, exists := pending.Files["index.json"]; exists {
		paths = append(paths, "index.json") // 索引最后发布。
	}
	for _, path := range paths {
		if err := m.writeJSON(filepath.Join(m.root, path), pending.Files[path]); err != nil {
			return false, err
		}
	}
	if err := m.fs.DeleteFile(filepath.Join(m.root, ".pending.json")); err != nil {
		return false, err
	}
	return true, nil
}

func (m *MemoryServer) writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := m.fs.CreateDirectory(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := m.fs.WriteFile(path+".tmp", data, 0o600); err != nil {
		return err
	}
	return m.fs.MoveFile(path+".tmp", path)
}
