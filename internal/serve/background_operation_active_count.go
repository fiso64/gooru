package serve

import core "gooru.local/gooru"

type backgroundOperationActiveCounter interface {
	CountActiveBackgroundOperations(bool) (int, error)
}

func (l *GooruLibrary) CountActiveBackgroundOperations(visibleOnly bool) (int, error) {
	return l.client.CountActiveBackgroundOperations(visibleOnly)
}

func (s *Server) activeBackgroundOperationCount(operations []core.BackgroundOperationState) (int, error) {
	if counter, ok := s.backgroundOperations.(backgroundOperationActiveCounter); ok {
		return counter.CountActiveBackgroundOperations(true)
	}

	// Keep test/legacy reader adapters source-compatible. Production uses
	// GooruLibrary above and therefore always takes the exact aggregate path.
	count := 0
	for _, operation := range operations {
		if operation.Visible && (operation.Status == core.BackgroundWorkPending || operation.Status == core.BackgroundWorkRunning) {
			count++
		}
	}
	return count, nil
}
