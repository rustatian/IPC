package shm

import "errors"

var ErrCStringCreation = errors.New("error converting Go string into the C string")