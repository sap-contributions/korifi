package tools

import (
	"cmp"
	"maps"
	"reflect"
	"slices"
)

func Uniq[S ~[]E, E cmp.Ordered](elements S) S {
	slices.Sort(elements)
	return slices.Compact(elements)
}

func EmptyOrContains[S ~[]E, E comparable](elements S, e E) bool {
	if len(elements) == 0 {
		return true
	}

	return slices.Contains(elements, e)
}

func NilOrEquals[E comparable](value *E, expectedValue E) bool {
	if value == nil {
		return true
	}

	return expectedValue == *value
}

func ZeroOrEquals[E comparable](value E, expectedValue E) bool {
	if reflect.ValueOf(value).IsZero() {
		return true
	}

	return value == expectedValue
}

func SetMapValue[K comparable, V any](m map[K]V, key K, value V) map[K]V {
	if m == nil {
		m = map[K]V{}
	}
	m[key] = value

	return m
}

func GetMapValue[K comparable, V any](m map[K]V, key K, defaultValue V) V {
	if value, found := m[key]; found {
		return value
	}
	return defaultValue
}

func MergeMaps[M ~map[K]V, K comparable, V any](a, b M) M {
	if len(a) == 0 {
		return b
	}

	if len(b) == 0 {
		return a
	}

	merged := make(M, max(len(a), len(b)))
	maps.Copy(merged, a)
	maps.Copy(merged, b)

	return merged
}
