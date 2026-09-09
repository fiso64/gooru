package serve

// CancelUnattachedHiddenBackgroundOperations releases producer admission
// reservations left behind before a durable task was attached. Callers must run
// this during startup, before accepting new producer requests.
func (l *GooruLibrary) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	return l.client.CancelUnattachedHiddenBackgroundOperations(kind)
}
