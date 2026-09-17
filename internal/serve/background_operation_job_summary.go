package serve

import "encoding/json"

type backgroundOperationOutcome string

const (
	backgroundOperationOutcomeSuccess        backgroundOperationOutcome = "success"
	backgroundOperationOutcomePartialSuccess backgroundOperationOutcome = "partial_success"
	backgroundOperationOutcomeError          backgroundOperationOutcome = "error"
)

type backgroundOperationDTOAlias BackgroundOperationDTO

// MarshalJSON adds the user-facing job summary without changing the durable
// lifecycle model or the operation-specific structured result. Lifecycle status
// answers whether durable work ran; outcome answers whether the requested items
// succeeded, partially succeeded, or all failed.
func (dto BackgroundOperationDTO) MarshalJSON() ([]byte, error) {
	outcome, affectedCount, failedCount := backgroundOperationJobSummary(dto)
	return json.Marshal(struct {
		backgroundOperationDTOAlias
		Outcome       backgroundOperationOutcome `json:"outcome,omitempty"`
		AffectedCount *int64                     `json:"affected_count,omitempty"`
		FailedCount   *int64                     `json:"failed_count,omitempty"`
	}{
		backgroundOperationDTOAlias: backgroundOperationDTOAlias(dto),
		Outcome:                     outcome,
		AffectedCount:               affectedCount,
		FailedCount:                 failedCount,
	})
}

func backgroundOperationJobSummary(dto BackgroundOperationDTO) (backgroundOperationOutcome, *int64, *int64) {
	switch dto.Status {
	case "failed":
		outcome := backgroundOperationOutcomeError
		if dto.Kind == backgroundUploadImportOperationKind && dto.ErrorCode == "payload_too_large" {
			zero := int64(0)
			one := int64(1)
			return outcome, &zero, &one
		}
		return outcome, nil, nil
	case "completed":
		// Continue below: completed durable work may still contain per-item
		// failures and therefore have a partial/error semantic outcome.
	default:
		return "", nil, nil
	}

	if outcome, ok := persistedBackgroundOperationOutcome(dto.ResultOutcome); ok {
		return outcome, dto.ResultAffectedCount, dto.ResultFailedCount
	}

	outcome := backgroundOperationOutcomeSuccess
	zero := int64(0)
	failedCount := &zero

	if dto.Kind == backgroundUploadImportOperationKind {
		var result struct {
			Files []struct {
				Status string `json:"status"`
			} `json:"files"`
		}
		if len(dto.Result) == 0 || json.Unmarshal(dto.Result, &result) != nil {
			return outcome, nil, failedCount
		}
		var imported, failed int64
		for _, file := range result.Files {
			switch file.Status {
			case "imported":
				imported++
			case "error":
				failed++
			}
		}
		affectedCount := imported
		failedValue := failed
		if failed > 0 {
			if failed == int64(len(result.Files)) {
				outcome = backgroundOperationOutcomeError
			} else {
				outcome = backgroundOperationOutcomePartialSuccess
			}
		}
		return outcome, &affectedCount, &failedValue
	}

	if dto.Kind == "tag_mutation" {
		var result struct {
			MatchedFiles int64 `json:"matched_files"`
		}
		if len(dto.Result) > 0 && json.Unmarshal(dto.Result, &result) == nil {
			affectedCount := result.MatchedFiles
			return outcome, &affectedCount, failedCount
		}
	}

	var result struct {
		AffectedCount *int64 `json:"affected_count"`
	}
	if len(dto.Result) > 0 && json.Unmarshal(dto.Result, &result) == nil && result.AffectedCount != nil && *result.AffectedCount >= 0 {
		return outcome, result.AffectedCount, failedCount
	}
	return outcome, nil, failedCount
}

func persistedBackgroundOperationOutcome(value string) (backgroundOperationOutcome, bool) {
	switch backgroundOperationOutcome(value) {
	case backgroundOperationOutcomeSuccess, backgroundOperationOutcomePartialSuccess, backgroundOperationOutcomeError:
		return backgroundOperationOutcome(value), true
	default:
		return "", false
	}
}
