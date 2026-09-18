package service

import "cc-052/internal/model"

// ConflictError 表示归一化后与已有档案冲突（409）。
// Existing 带出已经存在的是哪一家，供建档时当场指出。
type ConflictError struct {
	Kind     string      `json:"kind"` // duplicate_name | duplicate_cert
	Message  string      `json:"message"`
	Existing *model.Farm `json:"existing,omitempty"`
}

func (e *ConflictError) Error() string { return e.Message }

// ValidationError 输入不合法（400）。
type ValidationError struct {
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return e.Message }

func asConflict(err error) (*ConflictError, bool) {
	if ce, ok := err.(*ConflictError); ok {
		return ce, true
	}
	return nil, false
}
