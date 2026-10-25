package store

import "strings"

func Normalize(key string) string { return strings.ToLower(strings.TrimSpace(key)) }
