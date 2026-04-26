package sqlite

import "strings"

// IsUniqueConstraintError checks if the error is a SQLite UNIQUE constraint violation.
func IsUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// IsDuplicateColumnError checks if the error is a SQLite duplicate column error.
func IsDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate column")
}
