package main

import (
	"reflect"
	"testing"

	srvConfig "github.com/CHESSComputing/golib/config"
	ldap "github.com/CHESSComputing/golib/ldap"
)

// TestFinalBtrs tests finalBtrs function
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

// TestUpdateSpec tests updateSpec function
func TestUpdateSpec(t *testing.T) {
	srvConfig.Init()
	tests := []struct {
		name     string
		ispec    map[string]any
		attrs    ldap.Entry
		useCase  string
		expected map[string]any
	}{
		{
			name: "Search use-case with allowed btrs",
			ispec: map[string]any{
				"btr": []string{"btr1", "btr2", "btr3"},
			},
			attrs: ldap.Entry{
				Btrs: []string{"btr1", "btr3"},
			},
			useCase: "search",
			expected: map[string]any{
				"btr": map[string]any{"$in": []string{"btr1", "btr3"}},
			},
		},
		{
			name: "Filter use-case with multiple conditions",
			ispec: map[string]any{
				"category": "science",
			},
			attrs: ldap.Entry{
				Btrs: []string{"btr1", "btr3"},
			},
			useCase: "filter",
			expected: map[string]any{
				"$and": []map[string]any{
					{
						"$or": []map[string]any{
							{"category": "science"},
						},
					},
					{
						"btr": map[string]any{"$in": []string{"btr1", "btr3"}},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := updateSpec(test.ispec, test.attrs, test.useCase)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("Test %s failed. Expected %v, got %v", test.name, test.expected, result)
			}
		})
	}
}
