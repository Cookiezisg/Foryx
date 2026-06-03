package errors

import stderrors "errors"

// Error is the structured domain error: a semantic Kind, a stable wire Code, a
// human Message, optional structured Details (surfaced as N1 error.details), and
// an optional wrapped cause. transport reads these fields directly — no central
// error table, no per-package import fan-out.
//
// Error 是结构化 domain 错误：语义 Kind + 稳定 wire Code + 人类 Message +
// 可选结构化 Details（作为 N1 error.details）+ 可选 cause。transport 直接读这些
// 字段——无集中错误表、无逐包 import 扇出。
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Details map[string]any
	cause   error
}

// New builds a domain error with the given kind, stable wire code, and message.
// Package-level sentinels use it: var ErrX = New(KindNotFound, "X_NOT_FOUND", "x not found").
//
// New 用给定 kind / wire code / message 构造 domain 错误。包级 sentinel 用它构造。
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Error implements error; appends the cause when present.
//
// Error 实现 error 接口；有 cause 时附加。
func (e *Error) Error() string {
	if e.cause != nil {
		return e.Message + ": " + e.cause.Error()
	}
	return e.Message
}

// Unwrap exposes the wrapped cause for errors.Is/As traversal.
//
// Unwrap 暴露 cause 供 errors.Is/As 遍历。
func (e *Error) Unwrap() error { return e.cause }

// Is matches by Code, so a sentinel still equals its WithCause/WithDetails clones
// under errors.Is — keeping the sentinel-comparison habit while allowing wrapping.
//
// Is 按 Code 匹配，使 sentinel 与其 WithCause/WithDetails 副本在 errors.Is 下仍相等
// ——保留 sentinel 比较习惯，同时允许包裹。
func (e *Error) Is(target error) bool {
	var t *Error
	if !stderrors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// WithCause returns a copy wrapping cause (for logging / Unwrap); Is still matches the sentinel.
//
// WithCause 返回包裹 cause 的副本（供日志 / Unwrap）；Is 仍匹配 sentinel。
func (e *Error) WithCause(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// WithDetails returns a copy carrying structured details for the N1 error.details field.
//
// WithDetails 返回携带结构化 details 的副本（用于 N1 error.details）。
func (e *Error) WithDetails(details map[string]any) *Error {
	clone := *e
	clone.Details = details
	return &clone
}
