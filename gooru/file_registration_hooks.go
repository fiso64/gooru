package gooru

// FileRegistrationEvent describes content identities whose tracked locations
// were registered or refreshed by one tagging transaction. Hooks run after the
// domain writes but before commit so durable follow-up tasks can be persisted
// atomically with registration.
type FileRegistrationEvent struct {
	ContentHashes []string
}

// FileRegistrationHook builds durable follow-up work for a registration event.
// Hooks must not perform external side effects: an error aborts the surrounding
// database transaction.
type FileRegistrationHook func(FileRegistrationEvent) ([]BackgroundTaskRequest, error)

// SetFileRegistrationHooks replaces the hooks used for future registration
// transactions. Composition code should configure hooks before concurrent file
// mutations begin.
func (c *Client) SetFileRegistrationHooks(hooks ...FileRegistrationHook) {
	c.fileRegistrationHooks = append(c.fileRegistrationHooks[:0], hooks...)
}

func (c *Client) fileRegistrationBackgroundTasks(hashes []string) ([]BackgroundTaskRequest, error) {
	if len(hashes) == 0 || len(c.fileRegistrationHooks) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(hashes))
	unique := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		if hash == "" {
			continue
		}
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		unique = append(unique, hash)
	}
	if len(unique) == 0 {
		return nil, nil
	}
	event := FileRegistrationEvent{ContentHashes: unique}
	var tasks []BackgroundTaskRequest
	for _, hook := range c.fileRegistrationHooks {
		if hook == nil {
			continue
		}
		built, err := hook(event)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, built...)
	}
	return tasks, nil
}
