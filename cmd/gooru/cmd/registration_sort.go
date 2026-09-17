package cmd

import (
	"fmt"
	"strings"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func parseRegistrationSort(value string) (core.FileRegistrationSort, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "queue":
		return core.FileRegistrationSortQueue, nil
	case "reverse", "reverse-queue", "reverse_queue":
		return core.FileRegistrationSortReverseQueue, nil
	case "modtime", "mtime":
		return core.FileRegistrationSortModTime, nil
	default:
		return "", fmt.Errorf("invalid --sort %q: expected queue, reverse, or modtime", value)
	}
}

func tagFilesWithRegistrationSort(filePaths []string, tags []string, progressCb func(filePath string, err error), useMetadata bool, sortValue string) (types.TagOperationResult, error) {
	sort, err := parseRegistrationSort(sortValue)
	if err != nil {
		return types.TagOperationResult{}, err
	}
	return svc.TagFilesWithSort(filePaths, tags, progressCb, useMetadata, sort)
}

func setTagsForFilesWithRegistrationSort(filePaths []string, tags []string, progressCb func(filePath string, err error), useMetadata bool, sortValue string) (types.TagOperationResult, error) {
	sort, err := parseRegistrationSort(sortValue)
	if err != nil {
		return types.TagOperationResult{}, err
	}
	return svc.SetTagsForFilesWithSort(filePaths, tags, progressCb, useMetadata, sort)
}

const registrationSortHelp = "Order newly tracked files by added time: queue (last input newest), reverse (first input newest), or modtime"
