//go:build unix && !linux

package shm

import "errors"

// removePosixName is unreachable on non-Linux platforms because
// NewSharedMemoryPosix isn't compiled outside Linux. The stub exists so
// the kindPosix branch in Remove (shared code) links successfully.
func removePosixName(_ string) error {
	return errors.New("POSIX shared memory is only supported on Linux")
}
