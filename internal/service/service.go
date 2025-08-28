package service

import (
	"database/sql"
	"gooru/internal/database"
	"gooru/internal/hashing"
)

// Service encapsulates the core business logic.
	type Service struct {
	DB *sql.DB
}

// NewService creates a new Service.
func NewService(db *sql.DB) *Service {
	return &Service{DB: db}
}

// TagFile tags a file with the given tags.
func (s *Service) TagFile(filePath string, tags []string) error {
	hash, err := hashing.HashFile(filePath)
	if err != nil {
		return err
	}

	if err := database.AddContent(s.DB, hash); err != nil {
		return err
	}

	if err := database.AddLocation(s.DB, hash, filePath); err != nil {
		return err
	}

	for _, tagName := range tags {
		tagID, err := database.AddTag(s.DB, tagName)
		if err != nil {
			return err
		}

		if err := database.AssociateTag(s.DB, hash, tagID); err != nil {
			return err
		}
	}

	return nil
}

// GetTagsForFile retrieves all tags for a given file.
func (s *Service) GetTagsForFile(filePath string) ([]string, error) {
	hash, err := database.FindContent(s.DB, filePath)
	if err != nil {
		return nil, err
	}

	return database.GetTagsForContent(s.DB, hash)
}

// UntagFile untags a file with the given tags.
func (s *Service) UntagFile(filePath string, tags []string) error {
	hash, err := database.FindContent(s.DB, filePath)
	if err != nil {
		return err
	}

	for _, tagName := range tags {
		tagID, err := database.GetTag(s.DB, tagName)
		if err != nil {
			// If the tag doesn't exist, we can just ignore it
			continue
		}

		if err := database.DisassociateTag(s.DB, hash, tagID); err != nil {
			return err
		}
	}

	return nil
}

// ListAllFiles lists all files known to the system.
func (s *Service) ListAllFiles() ([]string, error) {
	return database.ListAllFiles(s.DB)
}

// ListFilesByTag lists all files associated with a given tag.
func (s *Service) ListFilesByTag(tag string) ([]string, error) {
	return database.ListFilesByTag(s.DB, tag)
}

// ListFilesByTagsAnd lists all files associated with a given set of tags (AND query).
func (s *Service) ListFilesByTagsAnd(tags []string) ([]string, error) {
	return database.ListFilesByTagsAnd(s.DB, tags)
}



