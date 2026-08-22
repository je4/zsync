package utils

import (
	"reflect"
	"testing"
)

func TestChunk(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		size     int
		expected [][]int
	}{
		{
			name:     "empty slice",
			input:    []int{},
			size:     2,
			expected: nil,
		},
		{
			name:     "size <= 0",
			input:    []int{1, 2, 3},
			size:     0,
			expected: nil,
		},
		{
			name:     "exact chunks",
			input:    []int{1, 2, 3, 4},
			size:     2,
			expected: [][]int{{1, 2}, {3, 4}},
		},
		{
			name:     "uneven chunks",
			input:    []int{1, 2, 3, 4, 5},
			size:     2,
			expected: [][]int{{1, 2}, {3, 4}, {5}},
		},
		{
			name:     "chunk size larger than slice",
			input:    []int{1, 2},
			size:     5,
			expected: [][]int{{1, 2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Chunk(tt.input, tt.size)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Chunk() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestExtractKeys(t *testing.T) {
	type user struct {
		ID   int
		Name string
	}
	users := []user{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}

	ids := ExtractKeys(users, func(u user) int { return u.ID })
	if !reflect.DeepEqual(ids, []int{1, 2}) {
		t.Errorf("ExtractKeys(ID) = %v, want [1, 2]", ids)
	}

	names := ExtractKeys(users, func(u user) string { return u.Name })
	if !reflect.DeepEqual(names, []string{"Alice", "Bob"}) {
		t.Errorf("ExtractKeys(Name) = %v, want ['Alice', 'Bob']", names)
	}
}

func TestFilter(t *testing.T) {
	numbers := []int{1, 2, 3, 4, 5, 6}
	evens := Filter(numbers, func(n int) bool { return n%2 == 0 })
	if !reflect.DeepEqual(evens, []int{2, 4, 6}) {
		t.Errorf("Filter() = %v, want [2, 4, 6]", evens)
	}
}

func TestMap(t *testing.T) {
	numbers := []int{1, 2, 3}
	doubled := Map(numbers, func(n int) int { return n * 2 })
	if !reflect.DeepEqual(doubled, []int{2, 4, 6}) {
		t.Errorf("Map() = %v, want [2, 4, 6]", doubled)
	}
}
