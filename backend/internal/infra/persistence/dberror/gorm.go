package dberror

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

type sqlStateError interface {
	SQLState() string
}

// IsRecordNotFound 判断底层错误是否表示记录不存在。
func IsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// IsUniqueConstraint 判断底层错误是否表示唯一约束冲突。
func IsUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var stateErr sqlStateError
	if errors.As(err, &stateErr) && stateErr.SQLState() == "23505" {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique constraint failed")
}

// IsForeignKeyConstraint reports whether a write violates referential integrity.
func IsForeignKeyConstraint(err error) bool {
	if err == nil {
		return false
	}
	var stateErr sqlStateError
	if errors.As(err, &stateErr) && stateErr.SQLState() == "23503" {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "violates foreign key constraint") ||
		strings.Contains(msg, "foreign key constraint failed")
}
