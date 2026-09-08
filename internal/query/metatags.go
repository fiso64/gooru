package query

import (
	"fmt"
	"strings"
)

const (
	MetaTagTagged           = "tagged"
	MetaTagFilenameContains = "filename_contains"
)

type MetaTagDefinition struct {
	Name          string
	Syntax        string
	Hint          string
	RequiresValue bool
}

type ParsedMetaTag struct {
	Definition MetaTagDefinition
	Value      string
}

var metaTagDefinitions = []MetaTagDefinition{
	{Name: MetaTagTagged, Syntax: "@tagged", Hint: "has tags", RequiresValue: false},
	{Name: MetaTagFilenameContains, Syntax: "@filename_contains:", Hint: "filename contains", RequiresValue: true},
}

// MetaTags returns the backend-owned catalog of reserved query syntax exposed by
// the autocomplete API. The historical name is kept for wire compatibility.
func MetaTags() []MetaTagDefinition {
	out := make([]MetaTagDefinition, 0, len(metaTagDefinitions)+32)
	out = append(out, metaTagDefinitions...)
	for _, field := range ReservedFields() {
		out = append(out, MetaTagDefinition{Name: field.Name, Syntax: field.Syntax, Hint: field.Hint, RequiresValue: true})
		for _, value := range field.Values {
			out = append(out, MetaTagDefinition{Name: field.Name, Syntax: field.Syntax + value, Hint: field.Hint, RequiresValue: false})
		}
	}
	return out
}

func ParseMetaTag(raw string) (ParsedMetaTag, error) {
	if !strings.HasPrefix(raw, "@") {
		return ParsedMetaTag{}, fmt.Errorf("%q is not a meta-tag", raw)
	}
	nameValue := strings.TrimPrefix(raw, "@")
	name, value, hasValue := strings.Cut(nameValue, ":")
	name = strings.ToLower(strings.TrimSpace(name))
	for _, definition := range metaTagDefinitions {
		if definition.Name != name {
			continue
		}
		if definition.RequiresValue {
			if !hasValue || strings.TrimSpace(value) == "" {
				return ParsedMetaTag{}, fmt.Errorf("meta-tag %s requires a value", definition.Syntax)
			}
			return ParsedMetaTag{Definition: definition, Value: value}, nil
		}
		if hasValue {
			return ParsedMetaTag{}, fmt.Errorf("meta-tag %s does not accept a value", definition.Syntax)
		}
		return ParsedMetaTag{Definition: definition}, nil
	}
	return ParsedMetaTag{}, fmt.Errorf("unsupported meta-tag @%s", name)
}
