package errors

import "strings"

type ErrorNotFound string

func (r ErrorNotFound) Error() string {
	return string(r)
}

type ErrorAlreadySynced string

func (r ErrorAlreadySynced) Error() string {
	return string(r)
}

type ErrorChecksumMismatch string

func (r ErrorChecksumMismatch) Error() string {
	return string(r)
}

type ErrorSetMarkFailed string

func (r ErrorSetMarkFailed) Error() string {
	return string(r)
}

type ErrorMultipleResults []string

func (r ErrorMultipleResults) Error() string {
	return "multiple results: " + strings.Join(r, " ")
}
