from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "gooru/tagging.go",
    '''\t// 4. Persist durable follow-up work in the same transaction as content registration.\n\tfor _, task := range tasks {\n\t\tif _, _, err := c.enqueueBackgroundTask(tx, task); err != nil {\n\t\t\treturn 0, nil, fmt.Errorf("failed to enqueue background task: %w", err)\n\t\t}\n\t}\n\n\t// 5. Persist producer recovery state and the exact success payload before\n\t// committing the domain mutation. A crash can therefore expose either the\n\t// pre-import state or the complete replayable import state, never a split.\n\tif stateBuilder != nil {\n\t\tstate, err := stateBuilder(int(affectedCount))\n\t\tif err != nil {\n\t\t\treturn 0, nil, fmt.Errorf("build background operation transaction state: %w", err)\n\t\t}\n\t\tif state.OperationID == "" && state.TaskID == "" {\n\t\t\treturn 0, nil, errors.New("background transaction state requires an operation or task id")\n\t\t}\n\t\tcheckpointJSON, err := json.Marshal(state.Checkpoint)\n\t\tif err != nil {\n\t\t\treturn 0, nil, fmt.Errorf("encode background transaction checkpoint: %w", err)\n\t\t}\n\t\tresultJSON, err := json.Marshal(state.Result)\n\t\tif err != nil {\n\t\t\treturn 0, nil, fmt.Errorf("encode background transaction result: %w", err)\n\t\t}\n\t\tif state.TaskID != "" {\n\t\t\tif err := setDatabaseBackgroundTaskState(c, tx, state.TaskID, checkpointJSON, resultJSON); err != nil {\n\t\t\t\treturn 0, nil, fmt.Errorf("persist background task transaction state: %w", err)\n\t\t\t}\n\t\t}\n\t\tif state.OperationID != "" {\n\t\t\tif err := setDatabaseBackgroundOperationState(c, tx, state.OperationID, checkpointJSON, resultJSON); err != nil {\n\t\t\t\treturn 0, nil, fmt.Errorf("persist background operation transaction state: %w", err)\n\t\t\t}\n\t\t}\n\t}\n''',
    '''\t// 4. Persist durable follow-up work and producer recovery state in the same\n\t// transaction as content registration.\n\tif err := c.persistTaggingFollowUpInTx(tx, tasks, stateBuilder, affectedCount); err != nil {\n\t\treturn 0, nil, err\n\t}\n''',
)

replace_once(
    "gooru/tagging.go",
    '''// executeTaggingTransaction performs all database writes for a tagging operation.\nfunc (c *Client) executeTaggingTransaction''',
    '''func (c *Client) persistTaggingFollowUpInTx(tx *databaseTx, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, affectedCount int64) error {\n\tfor _, task := range tasks {\n\t\tif _, _, err := c.enqueueBackgroundTask(tx, task); err != nil {\n\t\t\treturn fmt.Errorf("failed to enqueue background task: %w", err)\n\t\t}\n\t}\n\tif stateBuilder == nil {\n\t\treturn nil\n\t}\n\tstate, err := stateBuilder(int(affectedCount))\n\tif err != nil {\n\t\treturn fmt.Errorf("build background operation transaction state: %w", err)\n\t}\n\tif state.OperationID == "" && state.TaskID == "" {\n\t\treturn errors.New("background transaction state requires an operation or task id")\n\t}\n\tcheckpointJSON, err := json.Marshal(state.Checkpoint)\n\tif err != nil {\n\t\treturn fmt.Errorf("encode background transaction checkpoint: %w", err)\n\t}\n\tresultJSON, err := json.Marshal(state.Result)\n\tif err != nil {\n\t\treturn fmt.Errorf("encode background transaction result: %w", err)\n\t}\n\tif state.TaskID != "" {\n\t\tif err := setDatabaseBackgroundTaskState(c, tx, state.TaskID, checkpointJSON, resultJSON); err != nil {\n\t\t\treturn fmt.Errorf("persist background task transaction state: %w", err)\n\t\t}\n\t}\n\tif state.OperationID != "" {\n\t\tif err := setDatabaseBackgroundOperationState(c, tx, state.OperationID, checkpointJSON, resultJSON); err != nil {\n\t\t\treturn fmt.Errorf("persist background operation transaction state: %w", err)\n\t\t}\n\t}\n\treturn nil\n}\n\n// executeTaggingTransaction performs all database writes for a tagging operation.\nfunc (c *Client) executeTaggingTransaction''',
)

replace_once(
    "gooru/tagging_known_file_tags.go",
    '''import (\n\t"encoding/json"\n\t"errors"\n\t"fmt"\n''',
    '''import (\n\t"fmt"\n''',
)

replace_once(
    "gooru/tagging_known_file_tags.go",
    '''\tfor _, task := range tasks {\n\t\tif _, _, err := c.enqueueBackgroundTask(tx, task); err != nil {\n\t\t\treturn result, fmt.Errorf("failed to enqueue background task: %w", err)\n\t\t}\n\t}\n\tif stateBuilder != nil {\n\t\tstate, err := stateBuilder(int(affectedCount))\n\t\tif err != nil {\n\t\t\treturn result, fmt.Errorf("build background operation transaction state: %w", err)\n\t\t}\n\t\tif state.OperationID == "" && state.TaskID == "" {\n\t\t\treturn result, errors.New("background transaction state requires an operation or task id")\n\t\t}\n\t\tcheckpointJSON, err := json.Marshal(state.Checkpoint)\n\t\tif err != nil {\n\t\t\treturn result, fmt.Errorf("encode background transaction checkpoint: %w", err)\n\t\t}\n\t\tresultJSON, err := json.Marshal(state.Result)\n\t\tif err != nil {\n\t\t\treturn result, fmt.Errorf("encode background transaction result: %w", err)\n\t\t}\n\t\tif state.TaskID != "" {\n\t\t\tif err := setDatabaseBackgroundTaskState(c, tx, state.TaskID, checkpointJSON, resultJSON); err != nil {\n\t\t\t\treturn result, fmt.Errorf("persist background task transaction state: %w", err)\n\t\t\t}\n\t\t}\n\t\tif state.OperationID != "" {\n\t\t\tif err := setDatabaseBackgroundOperationState(c, tx, state.OperationID, checkpointJSON, resultJSON); err != nil {\n\t\t\t\treturn result, fmt.Errorf("persist background operation transaction state: %w", err)\n\t\t\t}\n\t\t}\n\t}\n''',
    '''\tif err := c.persistTaggingFollowUpInTx(tx, tasks, stateBuilder, affectedCount); err != nil {\n\t\treturn result, err\n\t}\n''',
)
