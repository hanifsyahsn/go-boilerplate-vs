package utility

// TruncateRunes returns s cut to at most n runes. Invalid UTF-8 bytes count
// as one rune each and are kept as they are (Postgres then refuses them, as
// it would have anyway).
func TruncateRunes(s string, n int) string {
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}
