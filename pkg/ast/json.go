package ast

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"unicode"
)

// ExtractJSONValue reads a scalar value at the specified path from JSON content.
func ExtractJSONValue(content []byte, path string) (string, error) {
	segments, err := ParsePath(path)
	if err != nil {
		return "", err
	}

	resolved, err := resolveJSONSegments(content, segments)
	if err != nil {
		return "", err
	}

	start, end, val, err := locateJSONScalar(content, resolved)
	if err != nil {
		return "", err
	}
	_ = start
	_ = end
	return val, nil
}

// MutateJSON updates a scalar value at the specified path while preserving original indentation and formatting.
func MutateJSON(content []byte, path string, newValue string) ([]byte, error) {
	segments, err := ParsePath(path)
	if err != nil {
		return nil, err
	}

	resolved, err := resolveJSONSegments(content, segments)
	if err != nil {
		return nil, err
	}

	start, end, _, err := locateJSONScalar(content, resolved)
	if err != nil {
		return nil, err
	}

	// Format newValue as JSON string or number/bool
	encodedVal, err := json.Marshal(newValue)
	if err != nil {
		return nil, fmt.Errorf("marshal new value: %w", err)
	}

	var result bytes.Buffer
	result.Write(content[:start])
	result.Write(encodedVal)
	result.Write(content[end:])

	return result.Bytes(), nil
}

// locateJSONScalar finds the start and end byte offsets of the scalar value at path.
func locateJSONScalar(content []byte, targetSegments []PathSegment) (int, int, string, error) {
	dec := json.NewDecoder(bytes.NewReader(content))

	// Stack to track current traversal depth: each level is either an object (with current key) or array (with current index)
	type scope struct {
		isMap bool
		key   string
		index int
	}

	var stack []scope

	getCurrentSegments := func() []PathSegment {
		var segs []PathSegment
		for _, s := range stack {
			if s.isMap {
				if s.key != "" {
					segs = append(segs, PathSegment{Type: SegmentField, Field: s.key})
				}
			} else {
				segs = append(segs, PathSegment{Type: SegmentIndex, Index: s.index})
			}
		}
		return segs
	}

	matchesTarget := func(current []PathSegment) bool {
		if len(current) != len(targetSegments) {
			return false
		}
		for i := range current {
			if current[i].Type != targetSegments[i].Type {
				return false
			}
			if current[i].Type == SegmentField && current[i].Field != targetSegments[i].Field {
				return false
			}
			if current[i].Type == SegmentIndex && current[i].Index != targetSegments[i].Index {
				return false
			}
		}
		return true
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, "", fmt.Errorf("json parse error: %w", err)
		}

		offset := int(dec.InputOffset())

		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, scope{isMap: true})
			case '}':
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
					if len(stack) > 0 {
						if stack[len(stack)-1].isMap {
							stack[len(stack)-1].key = ""
						} else {
							stack[len(stack)-1].index++
						}
					}
				}
			case '[':
				stack = append(stack, scope{isMap: false, index: 0})
			case ']':
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
					if len(stack) > 0 {
						if stack[len(stack)-1].isMap {
							stack[len(stack)-1].key = ""
						} else {
							stack[len(stack)-1].index++
						}
					}
				}
			}

		case string:
			if len(stack) > 0 && stack[len(stack)-1].isMap && stack[len(stack)-1].key == "" {
				// This string is a map key
				stack[len(stack)-1].key = t
				continue
			}

			// This is a string value
			currentSegs := getCurrentSegments()
			if matchesTarget(currentSegs) {
				start, end := findStringBounds(content, offset)
				return start, end, t, nil
			}

			// Clean up after consuming value
			if len(stack) > 0 {
				if stack[len(stack)-1].isMap {
					stack[len(stack)-1].key = ""
				} else {
					stack[len(stack)-1].index++
				}
			}

		default:
			// Number, bool, or null
			currentSegs := getCurrentSegments()
			valStr := fmt.Sprintf("%v", t)
			if t == nil {
				valStr = "null"
			}
			if matchesTarget(currentSegs) {
				start, end := findLiteralBounds(content, offset)
				return start, end, valStr, nil
			}

			if len(stack) > 0 {
				if stack[len(stack)-1].isMap {
					stack[len(stack)-1].key = ""
				} else {
					stack[len(stack)-1].index++
				}
			}
		}
	}

	return 0, 0, "", fmt.Errorf("path not found")
}

func findStringBounds(content []byte, endOffset int) (int, int) {
	// endOffset is right after the closing quote or following whitespace
	idx := endOffset - 1
	for idx >= 0 && unicode.IsSpace(rune(content[idx])) {
		idx--
	}
	end := idx + 1 // includes closing quote

	// Scan backwards to find matching opening quote
	idx--
	for idx >= 0 {
		if content[idx] == '"' {
			// check if escaped
			escapes := 0
			for e := idx - 1; e >= 0 && content[e] == '\\'; e-- {
				escapes++
			}
			if escapes%2 == 0 {
				return idx, end
			}
		}
		idx--
	}
	return 0, endOffset
}

func findLiteralBounds(content []byte, endOffset int) (int, int) {
	idx := endOffset - 1
	for idx >= 0 && unicode.IsSpace(rune(content[idx])) {
		idx--
	}
	end := idx + 1

	start := idx
	for start >= 0 {
		c := content[start]
		if unicode.IsSpace(rune(c)) || c == ':' || c == ',' || c == '[' || c == '{' {
			start++
			break
		}
		start--
	}
	if start < 0 {
		start = 0
	}
	return start, end
}

func resolveJSONSegments(content []byte, segments []PathSegment) ([]PathSegment, error) {
	hasSpecial := false
	for _, s := range segments {
		if(s.Type == SegmentFilter || (s.Type == SegmentIndex && s.Index < 0) || s.Type == SegmentWildcard) {
			hasSpecial = true
			break
		}
	}
	if(!hasSpecial) {
		return segments, nil
	}

	var root any
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, fmt.Errorf("json parse for path resolution: %w", err)
	}

	resolved := make([]PathSegment, 0, len(segments))
	curr := root

	for _, seg := range segments {
		switch seg.Type {
		case SegmentField:
			m, ok := curr.(map[string]any)
			if(!ok) {
				return nil, fmt.Errorf("expected object at %s", seg.Field)
			}
			val, exists := m[seg.Field]
			if(!exists) {
				return nil, fmt.Errorf("key not found: %s", seg.Field)
			}
			curr = val
			resolved = append(resolved, seg)

		case SegmentIndex:
			arr, ok := curr.([]any)
			if(!ok) {
				return nil, fmt.Errorf("expected array at index %d", seg.Index)
			}
			idx := seg.Index
			if(idx < 0) {
				idx = len(arr) + idx
			}
			if(idx < 0 || idx >= len(arr)) {
				return nil, fmt.Errorf("index out of bounds: %d (len=%d)", seg.Index, len(arr))
			}
			curr = arr[idx]
			resolved = append(resolved, PathSegment{Type: SegmentIndex, Index: idx})

		case SegmentFilter:
			arr, ok := curr.([]any)
			if(!ok) {
				return nil, fmt.Errorf("expected array for filter expression")
			}
			matchedIdx := -1
			for aIdx, item := range arr {
				if matchJSONFilter(item, seg.Filter) {
					matchedIdx = aIdx
					break
				}
			}
			if(matchedIdx == -1) {
				return nil, fmt.Errorf("no item matching filter expression")
			}
			curr = arr[matchedIdx]
			resolved = append(resolved, PathSegment{Type: SegmentIndex, Index: matchedIdx})

		case SegmentWildcard:
			if arr, ok := curr.([]any); ok && len(arr) > 0 {
				curr = arr[0]
				resolved = append(resolved, PathSegment{Type: SegmentIndex, Index: 0})
			} else if m, ok := curr.(map[string]any); ok && len(m) > 0 {
				for k, v := range m {
					curr = v
					resolved = append(resolved, PathSegment{Type: SegmentField, Field: k})
					break
				}
			} else {
				return nil, fmt.Errorf("wildcard target empty")
			}
		}
	}

	return resolved, nil
}

func matchJSONFilter(item any, expr *FilterExpr) bool {
	if(item == nil || expr == nil || len(expr.Predicates) < 1) {
		return false
	}
	for _, pred := range expr.Predicates {
		matched := evalJSONPredicate(item, pred)
		if(expr.IsOr && matched) {
			return true
		}
		if(!expr.IsOr && !matched) {
			return false
		}
	}
	return !expr.IsOr
}

func evalJSONPredicate(item any, pred FilterPredicate) bool {
	val, exists := extractJSONSubpathValue(item, pred.SubPath)
	if(pred.Op == OpExists) {
		if(pred.Negated) {
			return !exists || val == "" || val == "null" || val == "false"
		}
		return exists && val != "" && val != "null" && val != "false"
	}
	if(!exists) {
		return false
	}

	switch pred.Op {
	case OpEqual:
		return val == pred.Value
	case OpNotEqual:
		return val != pred.Value
	case OpLess:
		num1, err1 := strconv.ParseFloat(val, 64)
		num2, err2 := strconv.ParseFloat(pred.Value, 64)
		if(err1 == nil && err2 == nil) {
			return num1 < num2
		}
		return val < pred.Value
	case OpLessEqual:
		num1, err1 := strconv.ParseFloat(val, 64)
		num2, err2 := strconv.ParseFloat(pred.Value, 64)
		if(err1 == nil && err2 == nil) {
			return num1 <= num2
		}
		return val <= pred.Value
	case OpGreater:
		num1, err1 := strconv.ParseFloat(val, 64)
		num2, err2 := strconv.ParseFloat(pred.Value, 64)
		if(err1 == nil && err2 == nil) {
			return num1 > num2
		}
		return val > pred.Value
	case OpGreaterEqual:
		num1, err1 := strconv.ParseFloat(val, 64)
		num2, err2 := strconv.ParseFloat(pred.Value, 64)
		if(err1 == nil && err2 == nil) {
			return num1 >= num2
		}
		return val >= pred.Value
	default:
		return val == pred.Value
	}
}

func extractJSONSubpathValue(item any, subpath []string) (string, bool) {
	curr := item
	for _, p := range subpath {
		m, ok := curr.(map[string]any)
		if(!ok) {
			return "", false
		}
		val, exists := m[p]
		if(!exists) {
			return "", false
		}
		curr = val
	}
	if curr == nil {
		return "null", true
	}
	return fmt.Sprintf("%v", curr), true
}
