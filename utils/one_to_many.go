package utils

import (
	"math/rand"
)

// OneToMany is a map that associates one key with many values
// Note: This is NOT thread-safe. Caller must manage synchronization.
type OneToMany[K comparable, V comparable] struct {
	data map[K][]V
}

func NewOneToMany[K comparable, V comparable]() *OneToMany[K, V] {
	return &OneToMany[K, V]{
		data: make(map[K][]V),
	}
}

// Add associates a value with a key
func (otm *OneToMany[K, V]) Add(key K, value V) {
	otm.data[key] = append(otm.data[key], value)
}

// Remove removes a specific value from a key
// If it's the last value, removes the key entirely
func (otm *OneToMany[K, V]) Remove(key K, value V) {
	values, exists := otm.data[key]
	if !exists {
		return
	}

	// Find and remove the value
	for i, v := range values {
		if v == value {
			otm.data[key] = append(values[:i], values[i+1:]...)
			break
		}
	}

	// If no values left, delete the key
	if len(otm.data[key]) == 0 {
		delete(otm.data, key)
	}
}

// GetRandom returns a random value for the key
// Returns zero value and false if no values available
func (otm *OneToMany[K, V]) GetRandom(key K) (V, bool) {
	values, exists := otm.data[key]
	if !exists || len(values) == 0 {
		var zero V
		return zero, false
	}

	// Pick random value
	idx := rand.Intn(len(values))
	return values[idx], true
}

// GetAll returns all values for a key
// Returns nil if key doesn't exist
func (otm *OneToMany[K, V]) GetAll(key K) []V {
	values, exists := otm.data[key]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([]V, len(values))
	copy(result, values)
	return result
}

// RemoveValue removes a value from all keys
// Useful when a value is no longer valid
func (otm *OneToMany[K, V]) RemoveValue(value V) {
	for key := range otm.data {
		for i, v := range otm.data[key] {
			if v == value {
				otm.data[key] = append(otm.data[key][:i], otm.data[key][i+1:]...)
				break
			}
		}

		// Clean up empty keys
		if len(otm.data[key]) == 0 {
			delete(otm.data, key)
		}
	}
}

// Has checks if a key exists
func (otm *OneToMany[K, V]) Has(key K) bool {
	values, exists := otm.data[key]
	return exists && len(values) > 0
}

// Count returns the number of values for a key
func (otm *OneToMany[K, V]) Count(key K) int {
	return len(otm.data[key])
}
