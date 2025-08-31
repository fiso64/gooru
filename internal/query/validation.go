package query

import (
	"fmt"
	"strings"
)

var reservedTagKeys = map[string]struct{}{
	"ext":  {},
	"type": {},
}

// validateTagSyntax checks if a tag string conforms to the required syntactic format.
// It does not check for reserved keywords, making it suitable for validating query expressions.
func validateTagSyntax(tag string) error {
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}

	for _, r := range tag {
		// Printable ASCII is 32-126. Space (32) is disallowed.
		if r <= ' ' || r > '~' {
			return fmt.Errorf("tag '%s' contains invalid characters; only printable ASCII (no spaces) allowed", tag)
		}
	}

	// This function validates a single part (key or value).
	validatePart := func(part string, isKey bool) error {
		if isKey && part == "" {
			return fmt.Errorf("tag '%s' has an empty key part", tag)
		}
		if strings.HasPrefix(part, "-") || strings.HasPrefix(part, "!") || strings.HasPrefix(part, ":") {
			return fmt.Errorf("tag part '%s' in '%s' cannot start with '-', '!', or ':'", part, tag)
		}
		if strings.HasSuffix(part, "-") || strings.HasSuffix(part, "!") || strings.HasSuffix(part, ":") {
			return fmt.Errorf("tag part '%s' in '%s' cannot end with '-', '!', or ':'", part, tag)
		}
		return nil
	}

	parts := strings.SplitN(tag, ":", 2)
	if len(parts) == 1 {
		// Simple tag, treated as a key.
		return validatePart(parts[0], true)
	}

	// Key:Value tag.
	key, value := parts[0], parts[1]
	if err := validatePart(key, true); err != nil {
		return err
	}
	if err := validatePart(value, false); err != nil { // Value can be empty, so isKey=false
		return err
	}

	return nil
}

// ValidateTag checks if a tag is valid for a user to apply to a file.
// It checks both syntax and for reserved keywords.
func ValidateTag(tag string) error {
	if err := validateTagSyntax(tag); err != nil {
		return err
	}

	parsed := ParseTag(tag)
	if _, isReserved := reservedTagKeys[strings.ToLower(parsed.Key)]; isReserved {
		return fmt.Errorf("tag key '%s' is a reserved keyword for special queries and cannot be used for tagging", parsed.Key)
	}

	return nil
}

// ValidateTags applies ValidateTag to a slice of tags.
func ValidateTags(tags []string) error {
	for _, tag := range tags {
		if err := ValidateTag(tag); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAST recursively traverses a query AST and validates the syntax of all tag strings.
// It allows reserved keywords since they are valid in a query context.
func ValidateAST(expr *Expression) error {
	if expr == nil {
		return nil
	}
	for _, andTerm := range expr.Or {
		if err := validateAndTerm(andTerm); err != nil {
			return err
		}
	}
	return nil
}

func validateAndTerm(andTerm *AndTerm) error {
	if andTerm == nil {
		return nil
	}
	for _, term := range andTerm.And {
		if err := validateTerm(term); err != nil {
			return err
		}
	}
	return nil
}

func validateTerm(term *Term) error {
	if term == nil {
		return nil
	}
	return validateFactor(term.Factor)
}

func validateFactor(factor *Factor) error {
	if factor == nil {
		return nil
	}
	if factor.SubExpr != nil {
		return ValidateAST(factor.SubExpr)
	}
	if factor.Tag != nil {
		// Use the syntax-only validator for query expressions.
		return validateTagSyntax(*factor.Tag)
	}
	return nil
}