package transfers

func (m *Manager) Flush() error  { m.mu.Lock(); defer m.mu.Unlock(); return m.save() }
func (m *Manager) Limit() uint64 { m.mu.Lock(); defer m.mu.Unlock(); return m.limit }
