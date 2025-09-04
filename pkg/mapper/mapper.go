package mapper

// InvertMap меняет ключи и значения местами
// Не умеет обрабатывать одинаковые значения при разных ключах
func InvertMap[K, V comparable](in map[K]V) map[V]K {
	if len(in) == 0 {
		return nil
	}

	out := make(map[V]K, len(in))

	for k, v := range in {
		out[v] = k
	}

	return out
}
