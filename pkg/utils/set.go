package utils

import (
	"maps"
)

// Set represents a generic set of unique comparable elements.
type Set[T comparable] struct {
	elements map[T]struct{}
}

// NewSet creates a new Set initialized with optional elements.
func NewSet[T comparable](items ...T) *Set[T] {
	s := &Set[T]{
		elements: make(map[T]struct{}, len(items)),
	}
	s.Add(items...)
	return s
}

// Add inserts elements into the set.
func (s *Set[T]) Add(items ...T) {
	for _, item := range items {
		s.elements[item] = struct{}{}
	}
}

// Remove deletes elements from the set.
func (s *Set[T]) Remove(items ...T) {
	for _, item := range items {
		delete(s.elements, item)
	}
}

// Contains returns true if item is in the set.
func (s *Set[T]) Contains(item T) bool {
	if s == nil || s.elements == nil {
		return false
	}
	_, ok := s.elements[item]
	return ok
}

// Len returns the number of elements in the set.
func (s *Set[T]) Len() int {
	if s == nil {
		return 0
	}
	return len(s.elements)
}

// Slice returns all elements of the set as a slice.
func (s *Set[T]) Slice() []T {
	if s == nil || len(s.elements) == 0 {
		return nil
	}
	result := make([]T, 0, len(s.elements))
	for k := range s.elements {
		result = append(result, k)
	}
	return result
}

// Clone returns a shallow copy of the set.
func (s *Set[T]) Clone() *Set[T] {
	res := NewSet[T]()
	if s != nil && len(s.elements) > 0 {
		maps.Copy(res.elements, s.elements)
	}
	return res
}

// Union returns a new set containing elements from both s and other.
func (s *Set[T]) Union(other *Set[T]) *Set[T] {
	res := s.Clone()
	if other != nil {
		for k := range other.elements {
			res.elements[k] = struct{}{}
		}
	}
	return res
}

// Intersect returns a new set containing elements present in both s and other.
func (s *Set[T]) Intersect(other *Set[T]) *Set[T] {
	res := NewSet[T]()
	if s == nil || other == nil {
		return res
	}
	// Iterate over the smaller set for efficiency
	smaller, larger := s, other
	if len(smaller.elements) > len(larger.elements) {
		smaller, larger = larger, smaller
	}
	for k := range smaller.elements {
		if larger.Contains(k) {
			res.elements[k] = struct{}{}
		}
	}
	return res
}

// Difference returns a new set containing elements present in s but not in other.
func (s *Set[T]) Difference(other *Set[T]) *Set[T] {
	res := NewSet[T]()
	if s == nil {
		return res
	}
	if other == nil {
		return s.Clone()
	}
	for k := range s.elements {
		if !other.Contains(k) {
			res.elements[k] = struct{}{}
		}
	}
	return res
}

// Equal returns true if s and other contain exactly the same elements.
func (s *Set[T]) Equal(other *Set[T]) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	if len(s.elements) != len(other.elements) {
		return false
	}
	for k := range s.elements {
		if !other.Contains(k) {
			return false
		}
	}
	return true
}
