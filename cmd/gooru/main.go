package main

import (
	"fmt"
	"os"

	"gooru/internal/database"
	"gooru/internal/service"
)

func main() {
	db, err := database.InitDB("gooru.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	service := service.NewService(db)

	tags := []string{"test", "dummy"}
	if err := service.TagFile("test.txt", tags); err != nil {
		fmt.Fprintf(os.Stderr, "Error tagging file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully tagged 'test.txt' with:", tags)

	retrievedTags, err := service.GetTagsForFile("test.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting tags for file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Retrieved tags for 'test.txt':", retrievedTags)

	// Untag the file
	tagsToUntag := []string{"dummy"}
	if err := service.UntagFile("test.txt", tagsToUntag); err != nil {
		fmt.Fprintf(os.Stderr, "Error untagging file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully untagged 'test.txt' with:", tagsToUntag)

	retrievedTags, err = service.GetTagsForFile("test.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting tags for file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Retrieved tags for 'test.txt' after untagging:", retrievedTags)
}

