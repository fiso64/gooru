package gooru

import (
	"time"

	"gooru.local/internal/database"
)

type backgroundChangeTaskStoreBackend interface {
	RecoverExpiredBackgroundTaskLeases(time.Time) (int, error)
	ClaimNextBackgroundTask(resourceClass, workerID string, now time.Time, leaseDuration time.Duration) (database.BackgroundTask, bool, error)
	RenewBackgroundTaskLease(taskID, workerID string, now time.Time, leaseDuration time.Duration) (time.Time, error)
	CompleteBackgroundTask(taskID, workerID string, finishedAt time.Time) error
	FailBackgroundTask(taskID, workerID string, finishedAt, retryAt time.Time, errorCode, errorMessage string) (bool, error)
}

type backgroundChangeTaskStore struct {
	store  backgroundChangeTaskStoreBackend
	notify func()
}

func newBackgroundChangeTaskStore(client *Client) backgroundChangeTaskStore {
	return backgroundChangeTaskStore{store: client.store, notify: client.notifyBackgroundOperationChange}
}

func (s backgroundChangeTaskStore) RecoverExpiredBackgroundTaskLeases(now time.Time) (int, error) {
	recovered, err := s.store.RecoverExpiredBackgroundTaskLeases(now)
	if err == nil && recovered > 0 {
		s.notify()
	}
	return recovered, err
}

func (s backgroundChangeTaskStore) ClaimNextBackgroundTask(resourceClass, workerID string, now time.Time, leaseDuration time.Duration) (database.BackgroundTask, bool, error) {
	task, claimed, err := s.store.ClaimNextBackgroundTask(resourceClass, workerID, now, leaseDuration)
	if err == nil && claimed {
		s.notify()
	}
	return task, claimed, err
}

func (s backgroundChangeTaskStore) RenewBackgroundTaskLease(taskID, workerID string, now time.Time, leaseDuration time.Duration) (time.Time, error) {
	return s.store.RenewBackgroundTaskLease(taskID, workerID, now, leaseDuration)
}

func (s backgroundChangeTaskStore) CompleteBackgroundTask(taskID, workerID string, finishedAt time.Time) error {
	err := s.store.CompleteBackgroundTask(taskID, workerID, finishedAt)
	if err == nil {
		s.notify()
	}
	return err
}

func (s backgroundChangeTaskStore) FailBackgroundTask(taskID, workerID string, finishedAt, retryAt time.Time, errorCode, errorMessage string) (bool, error) {
	retrying, err := s.store.FailBackgroundTask(taskID, workerID, finishedAt, retryAt, errorCode, errorMessage)
	if err == nil {
		s.notify()
	}
	return retrying, err
}
