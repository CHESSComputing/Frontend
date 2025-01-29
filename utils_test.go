package main

import (
	"reflect"
	"testing"
)

func TestFinalBtrs(t *testing.T) {
	attrBtrs := []string{"A", "B", "C", "D"}

	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{
			name:     "Single valid string",
			input:    "A",
			expected: []string{"A"},
		},
		{
			name:     "Single invalid string",
			input:    "X",
			expected: []string{},
		},
		{
			name:     "List of mixed valid and invalid strings",
			input:    []string{"A", "X", "B"},
			expected: []string{"A", "B"},
		},
		{
			name: "MongoDB $or operator with valid and invalid values",
			input: map[string]any{
				"$or": []any{"A", "C", "X"},
			},
			expected: []string{"A", "C"},
		},
		{
			name: "MongoDB $in operator with valid and invalid values",
			input: map[string]any{
				"$in": []any{"B", "D", "Y"},
			},
			expected: []string{"B", "D"},
		},
		{
			name: "MongoDB $or operator with all invalid values",
			input: map[string]any{
				"$or": []any{"X", "Y", "Z"},
			},
			expected: []string{},
		},
		{
			name: "MongoDB $in operator with all invalid values",
			input: map[string]any{
				"$in": []any{"M", "N"},
			},
			expected: []string{},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Nil input",
			input:    nil,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := finalBtrs(tt.input, attrBtrs)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("finalBtrs(%v) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}
