package ssd

import "errors"

var (
	ErrNotFound = errors.New("ssd: block not found")
	ErrInjected = errors.New("ssd: injected fault")
)
