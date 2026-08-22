package utils

import (
	"slices"
	"testing"
)

func TestSetOperations(t *testing.T) {
	s := NewSet[string]("a", "b", "c")
	if s.Len() != 3 {
		t.Errorf("Len() = %d, want 3", s.Len())
	}
	if !s.Contains("a") || !s.Contains("b") || !s.Contains("c") {
		t.Errorf("Contains failed for initial elements")
	}
	if s.Contains("d") {
		t.Errorf("Contains returned true for absent element")
	}

	s.Add("d")
	if !s.Contains("d") || s.Len() != 4 {
		t.Errorf("Add failed")
	}

	s.Remove("b")
	if s.Contains("b") || s.Len() != 3 {
		t.Errorf("Remove failed")
	}

	slice := s.Slice()
	slices.Sort(slice)
	expectedSlice := []string{"a", "c", "d"}
	if !slices.Equal(slice, expectedSlice) {
		t.Errorf("Slice() = %v, want %v", slice, expectedSlice)
	}

	// Clone
	clone := s.Clone()
	if !clone.Equal(s) {
		t.Errorf("Clone() not equal to original")
	}

	// Union
	s2 := NewSet[string]("c", "d", "e")
	union := s.Union(s2)
	unionSlice := union.Slice()
	slices.Sort(unionSlice)
	if !slices.Equal(unionSlice, []string{"a", "c", "d", "e"}) {
		t.Errorf("Union() = %v, want [a c d e]", unionSlice)
	}

	// Intersect
	intersect := s.Intersect(s2)
	intersectSlice := intersect.Slice()
	slices.Sort(intersectSlice)
	if !slices.Equal(intersectSlice, []string{"c", "d"}) {
		t.Errorf("Intersect() = %v, want [c d]", intersectSlice)
	}

	// Difference
	diff := s.Difference(s2)
	diffSlice := diff.Slice()
	slices.Sort(diffSlice)
	if !slices.Equal(diffSlice, []string{"a"}) {
		t.Errorf("Difference() = %v, want [a]", diffSlice)
	}
}
