package network

import "sync"

// BufferPoolManager shares byte buffers across downloads while preserving size isolation.
type BufferPoolManager struct {
	mu    sync.Mutex
	pools map[int]*sync.Pool
}

func NewBufferPoolManager() *BufferPoolManager {
	return &BufferPoolManager{
		pools: make(map[int]*sync.Pool),
	}
}

func (m *BufferPoolManager) Get(size int) *sync.Pool {
	if size <= 0 {
		size = 1
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if pool, ok := m.pools[size]; ok {
		return pool
	}

	pool := &sync.Pool{
		New: func() any {
			buf := make([]byte, size)
			return &buf
		},
	}
	m.pools[size] = pool
	return pool
}
