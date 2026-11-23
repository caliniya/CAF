package Memory

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/caliniya/caf/utils/id"
)

func (m *MemoryServer) AddMemory(ctx context.Context, libraryID, data string, tags ...string) (string, error) {
	return m.AddNamedMemory(ctx, libraryID, "", data, tags...)
}

func (m *MemoryServer) AddNamedMemory(ctx context.Context, libraryID, name, data string, tags ...string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return "", err
	}
	r, err := m.dataLocked(libraryID)
	if err != nil {
		return "", err
	}
	key, err := id.New(id.Memory)
	if err != nil {
		return "", err
	}
	tag := description(append([]string{}, tags...))
	r.Data[key] = Memory{Name: name, Data: data, Tag: &tag}
	if err := m.saveDataLocked(ctx, libraryID, r); err != nil {
		return "", err
	}
	return key, nil
}

func (m *MemoryServer) UpdateMemory(ctx context.Context, libraryID, memoryID, data string, tags ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return err
	}
	r, err := m.dataLocked(libraryID)
	if err != nil {
		return err
	}
	if _, exists := r.Data[memoryID]; !exists {
		return errors.New("记忆单元不存在")
	}
	tag := description(append([]string{}, tags...))
	next := r.Data[memoryID]
	next.Data, next.Tag = data, &tag
	r.Data[memoryID] = next
	return m.saveDataLocked(ctx, libraryID, r)
}

// EditMemory 保存人工编辑的完整单元，保留指定时间，并归档修改前的副本。
func (m *MemoryServer) EditMemory(ctx context.Context, libraryID, memoryID string, next Memory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return err
	}
	if err := validateMemory(memoryID, next); err != nil {
		return err
	}
	r, err := m.dataLocked(libraryID)
	if err != nil {
		return err
	}
	if _, exists := r.Data[memoryID]; !exists {
		return errors.New("记忆单元不存在")
	}
	r.Data[memoryID] = cloneMemory(next)
	return m.saveDataChangesLocked(ctx, libraryID, r, memoryID)
}

func (m *MemoryServer) DeleteMemory(ctx context.Context, libraryID, memoryID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return err
	}
	r, err := m.dataLocked(libraryID)
	if err != nil {
		return err
	}
	if _, exists := r.Data[memoryID]; !exists {
		return errors.New("记忆单元不存在")
	}
	delete(r.Data, memoryID)
	return m.saveDataLocked(ctx, libraryID, r)
}

// SaveRecollection 保存完整 Recollection，自动对照旧内容并归档改变或删除的单元。
// 名称和介绍由 UpdateRecollection 修改；记忆时间由服务统一赋值。
func (m *MemoryServer) SaveRecollection(ctx context.Context, libraryID string, r Recollection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return err
	}
	return m.saveDataLocked(ctx, libraryID, cloneRecollection(r))
}

func (m *MemoryServer) saveDataLocked(ctx context.Context, libraryID string, r Recollection) error {
	return m.saveDataChangesLocked(ctx, libraryID, r, "")
}

func (m *MemoryServer) saveDataChangesLocked(ctx context.Context, libraryID string, r Recollection, editedMemoryID string) error {
	meta, err := m.metaLocked(libraryID)
	if err != nil {
		return err
	}
	if r.Name != meta.Name || r.Description != meta.Description || r.Data == nil {
		return errors.New("记忆库名称、介绍或数据容器无效")
	}
	old, err := m.dataLocked(libraryID)
	if err != nil {
		return err
	}
	changed, archived := false, map[string]Memory{}
	stamp := ""
	for key, next := range r.Data {
		if !id.Valid(key, id.Memory) {
			return errors.New("无效记忆单元 ID")
		}
		previous, exists := old.Data[key]
		// 普通保存统一取时；人工编辑只保留指定单元的时间。
		if exists && previous.Name == next.Name && previous.Data == next.Data && reflect.DeepEqual(previous.Tag, next.Tag) &&
			(key != editedMemoryID || previous.MemoryDate == next.MemoryDate) {
			r.Data[key] = cloneMemory(previous)
			continue
		}
		if exists {
			archived[key] = cloneMemory(previous)
		}
		if stamp == "" {
			stamp = m.now(ctx)
		}
		if key != editedMemoryID {
			next.MemoryDate = stamp
		}
		r.Data[key] = cloneMemory(next)
		changed = true
	}
	for key, previous := range old.Data {
		if _, exists := r.Data[key]; !exists {
			archived[key] = cloneMemory(previous)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if stamp == "" {
		stamp = m.now(ctx)
	}
	meta.Date = stamp
	files := map[string]any{meta.Name + "/data.json": r, meta.Name + "/meta.json": meta}
	if len(archived) > 0 {
		history, err := m.historyLocked(libraryID)
		if err != nil {
			return err
		}
		for key, previous := range archived {
			history[key] = append(history[key], previous)
		}
		files[meta.Name+"/history.json"] = history
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := m.commitLocked("", "", files); err != nil {
		return err
	}
	m.Recollections[libraryID] = cloneRecollection(r)
	return nil
}

func (m *MemoryServer) historyLocked(libraryID string) (map[string][]Memory, error) {
	meta, err := m.metaLocked(libraryID)
	if err != nil {
		return nil, err
	}
	var history map[string][]Memory
	if err := m.readJSON(filepath.Join(m.root, meta.Name, "history.json"), &history); err != nil {
		return nil, err
	}
	if history == nil {
		return nil, errors.New("记忆历史数据无效")
	}
	for key, copies := range history {
		if !id.Valid(key, id.Memory) {
			return nil, errors.New("历史记忆 ID 无效")
		}
		for _, copy := range copies {
			if err := validateMemory(key, copy); err != nil {
				return nil, err
			}
		}
	}
	return history, nil
}

// History 只在主动调用时读取历史。重复内容和重复时间均保留，按追加顺序返回。
func (m *MemoryServer) History(ctx context.Context, libraryID, memoryID string) ([]Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return nil, err
	}
	if !id.Valid(memoryID, id.Memory) {
		return nil, errors.New("无效记忆单元 ID")
	}
	history, err := m.historyLocked(libraryID)
	if err != nil {
		return nil, err
	}
	result := make([]Memory, 0, len(history[memoryID]))
	for _, copy := range history[memoryID] {
		result = append(result, cloneMemory(copy))
	}
	return result, nil
}

// RestoreMemory 使用历史数组下标选取副本，不需要版本 ID。
func (m *MemoryServer) RestoreMemory(ctx context.Context, libraryID, memoryID string, index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return err
	}
	history, err := m.historyLocked(libraryID)
	if err != nil {
		return err
	}
	copies := history[memoryID]
	if index < 0 || index >= len(copies) {
		return errors.New("历史记忆下标无效")
	}
	r, err := m.dataLocked(libraryID)
	if err != nil {
		return err
	}
	r.Data[memoryID] = cloneMemory(copies[index])
	return m.saveDataLocked(ctx, libraryID, r)
}

// Search 只检索调用方明确启用的库，不限制其作用域，不自动加入 Main。
// 空列表表示全部禁用；无查询词返回全部当前记忆；历史不参与检索。
func (m *MemoryServer) Search(ctx context.Context, enabledIDs []string, queries ...string) (map[string]map[string]Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return nil, err
	}
	result := make(map[string]map[string]Memory)
	for _, libraryID := range enabledIDs {
		if _, exists := result[libraryID]; exists {
			continue
		}
		r, err := m.dataLocked(libraryID)
		if err != nil {
			return nil, err
		}
		matches := make(map[string]Memory)
		for key, memory := range r.Data {
			match := len(queries) == 0
			for _, query := range queries {
				query = strings.TrimSpace(query)
				if query == "" {
					continue
				}
				if strings.Contains(strings.ToLower(memory.Data), strings.ToLower(query)) || strings.Contains(strings.ToLower(memory.Name), strings.ToLower(query)) {
					match = true
				}
				if memory.Tag != nil {
					for _, tag := range *memory.Tag {
						if strings.Contains(strings.ToLower(tag), strings.ToLower(query)) {
							match = true
						}
					}
				}
			}
			if match {
				matches[key] = cloneMemory(memory)
			}
		}
		result[libraryID] = matches
	}
	return result, nil
}
