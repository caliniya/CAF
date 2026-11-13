package Memory

type MemoryServer struct {
	Recollections *map[string]Recollection
}

func (m *MemoryServer) Init() *MemoryServer {

	return m
}

func (m *MemoryServer) Save() {

}
