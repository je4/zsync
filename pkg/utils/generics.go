package utils

// Chunk splits a slice of any type into chunks of the specified size.
// If size <= 0 or items is empty, it returns nil.
func Chunk[T any](items []T, size int) [][]T {
	if size <= 0 || len(items) == 0 {
		return nil
	}
	var chunks [][]T
	for size < len(items) {
		items, chunks = items[size:], append(chunks, items[0:size:size])
	}
	return append(chunks, items)
}

// ExtractKeys extracts a field of any comparable or arbitrary type from a slice of objects.
func ExtractKeys[T any, K comparable](items []T, selector func(T) K) []K {
	keys := make([]K, len(items))
	for i, item := range items {
		keys[i] = selector(item)
	}
	return keys
}

// Filter returns a new slice containing all elements of items that satisfy the predicate.
func Filter[T any](items []T, predicate func(T) bool) []T {
	var result []T
	for _, item := range items {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

// Map applies the transform function to each element of items and returns a slice of the results.
func Map[T any, R any](items []T, transform func(T) R) []R {
	result := make([]R, len(items))
	for i, item := range items {
		result[i] = transform(item)
	}
	return result
}
