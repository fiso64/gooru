package gooru

import "strings"

// FileRegistrationEvent describes content identities whose tracked locations
// were registered or refreshed by one tagging transaction. Hooks run after the
// domain writes but before commit so durable follow-up tasks can be persisted
// atomically with registration. OperationID carries an optional producer-owned
// operation lifetime, such as one browser upload spanning many registration
// transactions.
type FileRegistrationEvent struct {
	ContentHashes []string
	OperationID   string
}

// FileRegistrationHook builds durable follow-up work for a registration event.
// Hooks must not perform external side effects: an error aborts the surrounding
// database transaction.
type FileRegistrationHook func(FileRegistrationEvent) ([]BackgroundTaskRequest, error)

// SetFileRegistrationHooks replaces the hooks used for future registration
// transactions. Composition code should configure hooks before concurrent file
// mutations begin. Passing no hooks intentionally disables registration hooks;
// use ResetFileRegistrationHooks to restore the core defaults.
func (c *Client) SetFileRegistrationHooks(hooks ...FileRegistrationHook) {
	if len(hooks) == 0 {
		c.fileRegistrationHooks = []FileRegistrationHook{}
		return
	}
	c.fileRegistrationHooks = append([]FileRegistrationHook(nil), hooks...)
}

// ResetFileRegistrationHooks restores the core registration behavior used by a
// newly created Client. Keeping reset explicit avoids coupling the replacement
// setter's empty value to a feature-specific default.
func (c *Client) ResetFileRegistrationHooks() {
	c.fileRegistrationHooks = nil
}

func uniqueRegistrationHashes(hashes []string) []string {
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
	return unique
}

func (c *Client) fileRegistrationBackgroundTasks(hashes []string) ([]BackgroundTaskRequest, error) {
	return c.fileRegistrationBackgroundTasksForOperation(hashes, "")
}

func (c *Client) fileRegistrationBackgroundTasksForOperation(hashes []string, operationID string) ([]BackgroundTaskRequest, error) {
	unique := uniqueRegistrationHashes(hashes)
	if len(unique) == 0 {
		return nil, nil
	}
	operationID = strings.TrimSpace(operationID)

	hooks := c.fileRegistrationHooks
	if hooks == nil {
		// This function runs inside the registration transaction. Reading through
		// the pool observes the committed pre-transaction state, which is exactly
		// what we need here: ordinary retagging of already-known content must not
		// create a metadata job, while a genuinely new content identity is absent
		// until this transaction commits. A nil store is retained only for small
		// construction-level tests and behaves like an unclassified registration.
		if c.store != nil {
			existing, err := c.GetFileInfosByContentHashes(unique)
			if err != nil {
				return nil, err
			}
			if len(existing) == len(unique) {
				return nil, nil
			}
		}
		return mediaMetadataRegistrationTasksForProducerOperation(true, operationID)
	}
	if len(hooks) == 0 {
		return nil, nil
	}

	event := FileRegistrationEvent{ContentHashes: unique, OperationID: operationID}
	var tasks []BackgroundTaskRequest
	for _, hook := range hooks {
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
