package cmd

import (
	"fmt"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func parseRegistrationSort(value string) (core.FileRegistrationSort, error) {
	sort, err := core.ParseFileRegistrationSort(value)
	if err != nil {
		return "", fmt.Errorf("invalid --sort %q: expected newest-last, newest-first, or modtime", value)
	}
	return sort, nil
}

func validateRegistrationSortUsage(expressionMode, sortChanged bool) error {
	if expressionMode && sortChanged {
		return fmt.Errorf("--sort applies only to path-based registration, not expression mode")
	}
	return nil
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

const registrationSortHelp = "Order newly tracked files by added time: newest-last (last input newest), newest-first (first input newest), or modtime"
