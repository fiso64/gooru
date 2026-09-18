package query

// ExtractTags traverses a query AST and returns a unique list of all user-defined
// tag strings (e.g., "key:value", "key"). It ignores virtual tags like "ext:".
func ExtractTags(expr *Expression) []string {
    var tags []string
    seen := make(map[string]struct{})
    extract(expr, &tags, seen)
    return tags
}

// extract is the recursive helper for ExtractTags.
func extract(expr *Expression, tags *[]string, seen map[string]struct{}) {
    if expr == nil {
        return
    }
    for _, andTerm := range expr.Or {
        if andTerm == nil {
            continue
        }
        for _, term := range andTerm.And {
            if term == nil {
                continue
            }
            if term.Factor != nil {
                if term.Factor.Tag != nil {
                    tagStr := *term.Factor.Tag
                    if _, ok := seen[tagStr]; !ok {
                        // We only care about user tags for count optimization. Virtual tags
                        // like `ext:` or `type:` don't have stored counts.
                        parsed := ParseTag(tagStr)
                        if !IsReservedField(parsed.Key) {
                            *tags = append(*tags, tagStr)
                            seen[tagStr] = struct{}{}
                        }
                    }
                }
                if term.Factor.SubExpr != nil {
                    extract(term.Factor.SubExpr, tags, seen)
                }
            }
        }
    }
}
