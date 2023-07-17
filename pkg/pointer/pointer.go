package pointer

func To[T any](val T) *T {
	return &val
}

func ToOrNil[T comparable](val T) *T {
	var empty T
	if val == empty {
		return nil
	}
	return &val
}

func From[T any](val *T) T {
	var out T
	if val != nil {
		out = *val
	}
	return out
}
