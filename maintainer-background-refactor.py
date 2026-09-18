from pathlib import Path

path = Path("gooru/background.go")
src = path.read_text()
start = src.index("func (c *Client) EnqueueBackgroundTask(")
end = src.index("// enqueueBackgroundTask is the transaction-aware core primitive", start)
old = src[start:end]
if old.count("c.store.Begin()") != 2:
    raise SystemExit("expected two transaction branches")
if old.count("c.notifyBackgroundOperationChange()") != 3:
    raise SystemExit("unexpected notify count")

new = r'''func (c *Client) EnqueueBackgroundTask(request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	if request.Operation == nil {
		if request.OperationBinding != BackgroundOperationCreateNew {
			return BackgroundTask{}, false, fmt.Errorf("background operation binding requires an operation request")
		}
		if !request.CoalescePendingEquivalent {
			task, created, err = c.enqueueBackgroundTask(c.store.DB, request)
			if err == nil && created && task.OperationID != "" {
				c.notifyBackgroundOperationChange()
			}
			return task, created, err
		}
	} else {
		if request.OperationID != "" && request.OperationBinding != BackgroundOperationAssociateWithProducer {
			return BackgroundTask{}, false, fmt.Errorf("background task cannot declare both operation id and operation request")
		}
		if request.OperationBinding == BackgroundOperationAssociateWithProducer && request.OperationID == "" {
			return BackgroundTask{}, false, fmt.Errorf("associated background operation requires a producer operation id")
		}
	}

	return c.enqueueBackgroundTaskTransaction(request)
}

func (c *Client) enqueueBackgroundTaskTransaction(request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	requireCreated := request.Operation != nil && request.OperationBinding == BackgroundOperationCreateNew
	tx, err := c.store.Begin()
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("begin background task transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	task, created, err = c.enqueueBackgroundTask(tx, request)
	if err != nil {
		return BackgroundTask{}, false, err
	}
	if !created && requireCreated {
		return BackgroundTask{}, false, fmt.Errorf("new background operation child task dedupe key already active")
	}
	if err := tx.Commit(); err != nil {
		return BackgroundTask{}, false, fmt.Errorf("commit background task transaction: %w", err)
	}
	if created && task.OperationID != "" {
		c.notifyBackgroundOperationChange()
	}
	return task, created, nil
}

'''
path.write_text(src[:start] + new + src[end:])
