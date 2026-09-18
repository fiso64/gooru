package serve

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const (
	fileDownloadTTL            = 5 * time.Minute
	maxFileDownloadTickets     = 256
	maxFileDownloadRetainedIDs = 2_000_000
)

var errFileDownloadNotFound = errors.New("file download not found")

type fileDownloadTicket struct {
	OwnerID   string
	Target    bulkFileTarget
	ExpiresAt time.Time
	retained  int
}

type fileDownloadStore struct {
	mu             sync.Mutex
	tickets        map[string]fileDownloadTicket
	totalRetained  int
	maxTickets     int
	maxRetainedIDs int
	now            func() time.Time
}

func newFileDownloadStore() *fileDownloadStore {
	return &fileDownloadStore{
		tickets:        make(map[string]fileDownloadTicket),
		maxTickets:     maxFileDownloadTickets,
		maxRetainedIDs: maxFileDownloadRetainedIDs,
		now:            func() time.Time { return time.Now().UTC() },
	}
}

func (s *fileDownloadStore) create(ownerID string, target bulkFileTarget) (string, error) {
	idBytes := make([]byte, 24)
	if _, err := rand.Read(idBytes); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes)
	target = cloneBulkFileTarget(target)
	retained := len(target.FileIDs) + len(target.IncludeFileIDs) + len(target.ExcludeFileIDs)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	for len(s.tickets) >= s.maxTickets && len(s.tickets) > 0 {
		s.evictOldestLocked()
	}
	for s.totalRetained+retained > s.maxRetainedIDs && len(s.tickets) > 0 {
		s.evictOldestLocked()
	}
	s.tickets[id] = fileDownloadTicket{
		OwnerID:   ownerID,
		Target:    target,
		ExpiresAt: s.now().Add(fileDownloadTTL),
		retained:  retained,
	}
	s.totalRetained += retained
	return id, nil
}

func (s *fileDownloadStore) resolve(ownerID, id string) (bulkFileTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	ticket, ok := s.tickets[id]
	if !ok || ticket.OwnerID != ownerID {
		return bulkFileTarget{}, errFileDownloadNotFound
	}
	return cloneBulkFileTarget(ticket.Target), nil
}

func (s *fileDownloadStore) sweepLocked() {
	now := s.now()
	for id, ticket := range s.tickets {
		if !ticket.ExpiresAt.After(now) {
			s.deleteLocked(id, ticket)
		}
	}
}

func (s *fileDownloadStore) evictOldestLocked() {
	var oldestID string
	var oldest fileDownloadTicket
	for id, ticket := range s.tickets {
		if oldestID == "" || ticket.ExpiresAt.Before(oldest.ExpiresAt) {
			oldestID = id
			oldest = ticket
		}
	}
	if oldestID != "" {
		s.deleteLocked(oldestID, oldest)
	}
}

func (s *fileDownloadStore) deleteLocked(id string, ticket fileDownloadTicket) {
	delete(s.tickets, id)
	s.totalRetained -= ticket.retained
	if s.totalRetained < 0 {
		s.totalRetained = 0
	}
}

func cloneBulkFileTarget(target bulkFileTarget) bulkFileTarget {
	target.FileIDs = append([]string(nil), target.FileIDs...)
	target.IncludeFileIDs = append([]string(nil), target.IncludeFileIDs...)
	target.ExcludeFileIDs = append([]string(nil), target.ExcludeFileIDs...)
	return target
}
