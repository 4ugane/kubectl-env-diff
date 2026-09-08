package model

import (
	"sort"

	"k8s.io/apimachinery/pkg/api/resource"
)

// ResourceList maps a resource name to a quantity. Quantities are stored parsed
// rather than as strings so that 1Gi and 1024Mi compare equal.
type ResourceList map[string]resource.Quantity

// Equal reports whether two lists describe the same amounts. A nil list and an
// empty list are equal, but a nil list is NOT equal to one containing an
// explicit zero — absent and zero are different states.
func (r ResourceList) Equal(other ResourceList) bool {
	if len(r) != len(other) {
		return false
	}
	for name, a := range r {
		b, ok := other[name]
		if !ok || a.Cmp(b) != 0 {
			return false
		}
	}
	return true
}

// Keys returns resource names in sorted order. Callers must use this rather
// than ranging the map, because Go map iteration order is random and would make
// output nondeterministic.
func (r ResourceList) Keys() []string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Get returns the display string for a resource and whether it was present.
func (r ResourceList) Get(name string) (string, bool) {
	qty, ok := r[name]
	if !ok {
		return "", false
	}
	return qty.String(), true
}
