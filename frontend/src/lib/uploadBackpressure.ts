export const uploadJobStatusBatchSize = 64;

// Upload job status normally refreshes at the UI progress cadence. Once the
// browser admission window is full, refresh promptly enough that a completed
// import releases capacity instead of turning the polling interval into the
// dominant cost for fast small-file batches.
export const uploadJobStatusRefetchMs = 700;
export const uploadBackpressuredJobStatusRefetchMs = 50;
