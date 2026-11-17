package Memory

import "context"

var MemoryPath string = "/memory/"

type MemoryServer struct {
	Recollections *map[string]Recollection `json:"data"`
}

// Close implements [Backend.Server].
func (m *MemoryServer) Close(ctx context.Context) error {
	panic("unimplemented")
}

// Init implements [Backend.Server].
func (m *MemoryServer) Init(ctx context.Context) error {
	panic("unimplemented")
}

// Start implements [Backend.Server].
func (m *MemoryServer) Start(ctx context.Context) error {
	panic("unimplemented")
}
