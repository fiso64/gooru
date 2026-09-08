package query

import "strings"

// Query syntax convention: key:value denotes a reserved field selector;
// @name[:argument] denotes a query predicate/operator.
type ReservedFieldDefinition struct {
	Name   string
	Syntax string
	Hint   string
	Values []string
}

var reservedFieldDefinitions = []ReservedFieldDefinition{
	{Name: "type", Syntax: "type:", Hint: "media type", Values: []string{"photo", "video", "gif", "audio", "other"}},
	{Name: "ext", Syntax: "ext:", Hint: "file extension", Values: []string{"jpg", "jpeg", "png", "gif", "webp", "avif", "heic", "heif", "mp4", "webm", "mov", "mkv", "mp3", "flac", "ogg", "wav", "cbz"}},
}

func ReservedFields() []ReservedFieldDefinition {
	out := make([]ReservedFieldDefinition, len(reservedFieldDefinitions))
	for i, definition := range reservedFieldDefinitions {
		out[i] = definition
		out[i].Values = append([]string(nil), definition.Values...)
	}
	return out
}

func IsReservedField(name string) bool {
	candidate := strings.ToLower(strings.TrimSpace(name))
	for _, definition := range reservedFieldDefinitions {
		if definition.Name == candidate {
			return true
		}
	}
	return false
}
