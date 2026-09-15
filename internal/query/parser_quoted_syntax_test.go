package query

import "testing"

func TestParsePreservesQuotedUserFriendlySyntax(t *testing.T) {
	tests := []string{
		"@filename_contains:rock or roll",
		"@filename_contains:research and development",
		"@filename_contains:not a drill",
		"@filename_contains:bang! amp&",
		"@filename_contains:type:img",
	}

	for _, want := range tests {
		t.Run(want, func(t *testing.T) {
			expr, err := Parse(`"` + want + `"`)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(expr.Or) != 1 || len(expr.Or[0].And) != 1 {
				t.Fatalf("unexpected expression shape: %#v", expr)
			}
			factor := expr.Or[0].And[0].Factor
			if factor == nil || factor.Tag == nil {
				t.Fatalf("expected quoted tag factor, got %#v", factor)
			}
			if got := *factor.Tag; got != want {
				t.Fatalf("quoted tag = %q, want %q", got, want)
			}
		})
	}
}

func TestParseStillNormalizesUnquotedUserFriendlySyntax(t *testing.T) {
	expr, err := Parse("alpha and beta or not gamma")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(expr.Or) != 2 {
		t.Fatalf("OR terms = %d, want 2", len(expr.Or))
	}
	if len(expr.Or[0].And) != 2 {
		t.Fatalf("first AND terms = %d, want 2", len(expr.Or[0].And))
	}
	if len(expr.Or[1].And) != 1 || !expr.Or[1].And[0].Not {
		t.Fatalf("second OR term did not preserve NOT shorthand: %#v", expr.Or[1])
	}
}

func TestParseStillExpandsUnquotedVirtualTypeTags(t *testing.T) {
	expr, err := Parse("type:img")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(expr.Or) != 1 || len(expr.Or[0].And) != 1 {
		t.Fatalf("unexpected expression shape: %#v", expr)
	}
	factor := expr.Or[0].And[0].Factor
	if factor == nil || factor.SubExpr == nil {
		t.Fatalf("type:img was not expanded to a grouped expression: %#v", factor)
	}
}
