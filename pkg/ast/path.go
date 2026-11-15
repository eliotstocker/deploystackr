package ast

import (
	"fmt"
	"strconv"
	"strings"
)

// PathSegmentType indicates whether a segment is a field, index, wildcard, or filter.
type PathSegmentType int

const (
	SegmentField PathSegmentType = iota
	SegmentIndex
	SegmentWildcard
	SegmentFilter
)

type FilterOp string

const (
	OpEqual        FilterOp = "=="
	OpNotEqual     FilterOp = "!="
	OpLess         FilterOp = "<"
	OpLessEqual    FilterOp = "<="
	OpGreater      FilterOp = ">"
	OpGreaterEqual FilterOp = ">="
	OpExists       FilterOp = "exists"
)

// FilterPredicate represents a single test condition within a filter expression.
type FilterPredicate struct {
	Field   string
	SubPath []string
	Op      FilterOp
	Value   string
	Negated bool
}

// FilterExpr represents an RFC 9535 filter expression `[? <expr>]`.
type FilterExpr struct {
	Predicates []FilterPredicate
	IsOr       bool
}

// PathSegment represents one step in an RFC 9535 JSONPath traversal.
type PathSegment struct {
	Type   PathSegmentType
	Field  string
	Index  int
	Filter *FilterExpr
}

// ParsePath converts a path string into a slice of PathSegments according to RFC 9535.
// Supports standard syntax like "$.image.tag", "spec.template.spec.containers[0].image",
// negative indices "containers[-1].image", and filter expressions:
// "$.spec.template.spec.containers[?@.name == 'api'].image"
// "$.images[?@.name == 'ghcr.io/org/api-service'].newTag"
// and shorthand "images[name=api].newTag".
func ParsePath(path string) ([]PathSegment, error) {
	clean := strings.TrimSpace(path)
	clean = strings.TrimPrefix(clean, "$")
	clean = strings.TrimPrefix(clean, ".")

	if(clean == "") {
		return nil, fmt.Errorf("empty path selector")
	}

	var segments []PathSegment
	var current strings.Builder
	inBracket := false

	flushCurrent := func() {
		if(current.Len() > 0) {
			f := current.String()
			current.Reset()
			if(f == "*") {
				segments = append(segments, PathSegment{Type: SegmentWildcard})
			} else {
				segments = append(segments, PathSegment{
					Type:  SegmentField,
					Field: f,
				})
			}
		}
	}

	runes := []rune(clean)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		switch {
		case ch == '.':
			if(!inBracket) {
				flushCurrent()
			} else {
				current.WriteRune(ch)
			}
		case ch == '[':
			flushCurrent()
			inBracket = true
		case ch == ']':
			inBracket = false
			raw := current.String()
			current.Reset()
			seg, err := parseBracketContent(raw)
			if(err != nil) {
				return nil, err
			}
			segments = append(segments, seg)
		default:
			current.WriteRune(ch)
		}
	}
	flushCurrent()

	if(len(segments) < 1) {
		return nil, fmt.Errorf("invalid path: %s", path)
	}

	return segments, nil
}

func parseBracketContent(raw string) (PathSegment, error) {
	trimmed := strings.TrimSpace(raw)

	// Wildcard [*]
	if(trimmed == "*") {
		return PathSegment{Type: SegmentWildcard}, nil
	}

	// Filter expression: starts with '?' or contains '==' / '!=' / '@.' or shorthand 'name='
	if(strings.HasPrefix(trimmed, "?") || strings.Contains(trimmed, "@.") ||
		(strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "'") && !strings.HasPrefix(trimmed, "\""))) {
		filter, err := parseFilterExpr(trimmed)
		if(err != nil) {
			return PathSegment{}, fmt.Errorf("invalid filter expression in [%s]: %w", raw, err)
		}
		return PathSegment{
			Type:   SegmentFilter,
			Filter: filter,
		}, nil
	}

	// Number index e.g. [0] or [-1]
	cleanVal := strings.Trim(trimmed, ` '"`)
	if idx, err := strconv.Atoi(cleanVal); err == nil {
		return PathSegment{
			Type:  SegmentIndex,
			Index: idx,
		}, nil
	}

	// Quoted field name e.g. ['foo']
	return PathSegment{
		Type:  SegmentField,
		Field: cleanVal,
	}, nil
}

func parseFilterExpr(raw string) (*FilterExpr, error) {
	exprStr := strings.TrimSpace(raw)
	exprStr = strings.TrimPrefix(exprStr, "?")
	exprStr = strings.TrimSpace(exprStr)

	// Unwrap outer parentheses if present: (?(@.name == 'api'))
	if(strings.HasPrefix(exprStr, "(") && strings.HasSuffix(exprStr, ")")) {
		exprStr = strings.TrimSpace(exprStr[1 : len(exprStr)-1])
	}

	isOr := false
	var clauses []string
	if(strings.Contains(exprStr, "||")) {
		isOr = true
		clauses = strings.Split(exprStr, "||")
	} else if(strings.Contains(exprStr, "&&")) {
		clauses = strings.Split(exprStr, "&&")
	} else {
		clauses = []string{exprStr}
	}

	var preds []FilterPredicate
	for _, clause := range clauses {
		c := strings.TrimSpace(clause)
		if(c == "") {
			continue
		}
		pred, err := parseSinglePredicate(c)
		if(err != nil) {
			return nil, err
		}
		preds = append(preds, *pred)
	}

	if(len(preds) < 1) {
		return nil, fmt.Errorf("empty filter expression")
	}

	return &FilterExpr{
		Predicates: preds,
		IsOr:       isOr,
	}, nil
}

func parseSinglePredicate(clause string) (*FilterPredicate, error) {
	c := strings.TrimSpace(clause)
	if(strings.HasPrefix(c, "(") && strings.HasSuffix(c, ")")) {
		c = strings.TrimSpace(c[1 : len(c)-1])
	}

	// Determine operator
	ops := []struct {
		str FilterOp
		sep string
	}{
		{OpEqual, "=="},
		{OpNotEqual, "!="},
		{OpLessEqual, "<="},
		{OpGreaterEqual, ">="},
		{OpLess, "<"},
		{OpGreater, ">"},
		{OpEqual, "="}, // loose shorthand [name=api]
	}

	for _, opInfo := range ops {
		if idx := strings.Index(c, opInfo.sep); idx > -1 {
			lhs := strings.TrimSpace(c[:idx])
			rhs := strings.TrimSpace(c[idx+len(opInfo.sep):])

			field, subpath := cleanPathIdentifier(lhs)
			val := cleanLiteralValue(rhs)

			return &FilterPredicate{
				Field:   field,
				SubPath: subpath,
				Op:      opInfo.str,
				Value:   val,
			}, nil
		}
	}

	// Existence check: [?@.field] or [?!@.field]
	negated := false
	if(strings.HasPrefix(c, "!")) {
		negated = true
		c = strings.TrimSpace(c[1:])
	}

	field, subpath := cleanPathIdentifier(c)
	if(field == "") {
		return nil, fmt.Errorf("invalid predicate: %s", clause)
	}

	return &FilterPredicate{
		Field:   field,
		SubPath: subpath,
		Op:      OpExists,
		Negated: negated,
	}, nil
}

func cleanPathIdentifier(s string) (string, []string) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	s = strings.TrimPrefix(s, ".")
	s = strings.Trim(s, ` '"`)

	parts := strings.Split(s, ".")
	var subpath []string
	for _, p := range parts {
		clean := strings.Trim(strings.TrimSpace(p), ` '"[]`)
		if(clean != "") {
			subpath = append(subpath, clean)
		}
	}

	if(len(subpath) < 1) {
		return "", nil
	}
	return subpath[len(subpath)-1], subpath
}

func cleanLiteralValue(s string) string {
	s = strings.TrimSpace(s)
	if(len(s) >= 2) {
		if((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
			return s[1 : len(s)-1]
		}
	}
	return s
}
