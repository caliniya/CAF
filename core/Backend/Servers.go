package Backend

func (s *Servers) Get[T *Server](name string) (T, error) {
	var temp T
	return temp, nil
}

type Servers map[string]Server

type Server interface {
	Init()
}
