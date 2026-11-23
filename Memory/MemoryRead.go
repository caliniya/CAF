package Memory

import (
	"context"
	"errors"

	"github.com/caliniya/caf/utils/id"
)

// GetMemory 读取单条当前记忆，返回独立副本；历史需要显式调用 History。
func (m *MemoryServer) GetMemory(ctx context.Context, libraryID, memoryID string) (*Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return nil, err
	}
	if !id.Valid(memoryID, id.Memory) {
		return nil, errors.New("无效记忆单元 ID")
	}
	r, err := m.dataLocked(libraryID)
	if err != nil {
		return nil, err
	}
	value, exists := r.Data[memoryID]
	if !exists {
		return nil, errors.New("记忆单元不存在")
	}
	copy := cloneMemory(value)
	return &copy, nil
}
