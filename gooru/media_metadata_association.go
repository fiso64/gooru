package gooru

import "strings"

// mediaMetadataRegistrationTasksForAssociatedProducerOperation builds the
// producer-aware metadata wake. The sweep is a separate logical operation
// associated with the producer, so it never consumes producer progress slots.
// Every producer reuses one auxiliary metadata operation until that producer is
// terminal; registrations without a producer retain the standalone sweep path.
func mediaMetadataRegistrationTasksForAssociatedProducerOperation(hasRegistrations bool, producerOperationID string) ([]BackgroundTaskRequest, error) {
	producerOperationID = strings.TrimSpace(producerOperationID)
	if producerOperationID == "" {
		return mediaMetadataRegistrationTasks(hasRegistrations)
	}
	if !hasRegistrations {
		return nil, nil
	}
	wake, err := newMediaMetadataSweepTaskRequest()
	if err != nil {
		return nil, err
	}
	wake.OperationID = producerOperationID
	wake.Operation = &BackgroundOperationRequest{
		Kind:    BackgroundMediaMetadataSweepOperationKind,
		Visible: true,
	}
	wake.OperationBinding = BackgroundOperationAssociateWithProducer
	wake.CoalescePendingEquivalent = true
	wake.InputKey = mediaMetadataRegistrationInputKey
	return []BackgroundTaskRequest{wake}, nil
}
