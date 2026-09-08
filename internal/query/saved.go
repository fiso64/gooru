package query

import (
	"fmt"
	"strconv"
	"strings"
)

// SavedSearchResolver resolves a saved-search name to its stored query expression.
type SavedSearchResolver func(name string) (string, error)

// ExpandSavedSearches replaces @saved:<name> factors with parsed sub-expressions.
// Expansion happens on the AST so the saved query remains grouped as one factor
// and therefore cannot change the precedence of the surrounding expression.
func ExpandSavedSearches(expr *Expression, resolve SavedSearchResolver) error {
	if resolve == nil {
		return fmt.Errorf("saved search resolver is required")
	}
	return expandSavedSearches(expr, resolve, nil)
}

func expandSavedSearches(expr *Expression, resolve SavedSearchResolver, stack []string) error {
	if expr == nil {
		return nil
	}
	for _, andTerm := range expr.Or {
		if andTerm == nil {
			continue
		}
		for _, term := range andTerm.And {
			if term == nil || term.Factor == nil {
				continue
			}
			factor := term.Factor
			if factor.SubExpr != nil {
				if err := expandSavedSearches(factor.SubExpr, resolve, stack); err != nil {
					return err
				}
				continue
			}
			if factor.Tag == nil || !strings.HasPrefix(strings.ToLower(*factor.Tag), "@saved:") {
				continue
			}
			meta, err := ParseMetaTag(*factor.Tag)
			if err != nil || meta.Definition.Name != MetaTagSaved {
				if err != nil {
					return err
				}
				continue
			}
			name := strings.TrimSpace(meta.Value)
			key := strings.ToLower(name)
			for _, active := range stack {
				if active == key {
					cycle := append(append([]string(nil), stack...), key)
					return fmt.Errorf("saved search cycle: %s", strings.Join(cycle, " -> "))
				}
			}
			stored, err := resolve(name)
			if err != nil {
				return fmt.Errorf("resolve saved search %q: %w", name, err)
			}

			// Empty saved searches are valid and mean "all files". Represent that
			// truth value using existing query syntax so expansion remains safely
			// round-trippable through Format -> Parse without adding a user-visible
			// internal meta-tag. Every file is either tagged or not tagged.
			if strings.TrimSpace(stored) == "" {
				stored = "(@tagged | -@tagged)"
			}
			sub, err := Parse(stored)
			if err != nil {
				return fmt.Errorf("parse saved search %q: %w", name, err)
			}
			if err := expandSavedSearches(sub, resolve, append(stack, key)); err != nil {
				return err
			}
			factor.Tag = nil
			factor.SubExpr = sub
		}
	}
	return nil
}

// Format renders an AST as a canonical, fully grouped query expression. Tags
// are quoted so meta-tag values containing spaces remain round-trippable.
func Format(expr *Expression) string {
	if expr == nil {
		return ""
	}
	orTerms := make([]string, 0, len(expr.Or))
	for _, andTerm := range expr.Or {
		if andTerm == nil {
			continue
		}
		terms := make([]string, 0, len(andTerm.And))
		for _, term := range andTerm.And {
			if term == nil || term.Factor == nil {
				continue
			}
			factor := ""
			switch {
			case term.Factor.SubExpr != nil:
				factor = "(" + Format(term.Factor.SubExpr) + ")"
			case term.Factor.Tag != nil:
				factor = strconv.Quote(*term.Factor.Tag)
			}
			if factor == "" {
				continue
			}
			if term.Not {
				factor = "-" + factor
			}
			terms = append(terms, factor)
		}
		if len(terms) > 0 {
			orTerms = append(orTerms, strings.Join(terms, " "))
		}
	}
	return strings.Join(orTerms, " | ")
}
