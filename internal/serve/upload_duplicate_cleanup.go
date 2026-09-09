package serve

func discardDuplicateUpload(file StagedUpload, trackedAtPath bool) {
	if trackedAtPath {
		return
	}
	removeRejectedStagedUpload(file)
}
