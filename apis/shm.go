package apis

// SharedMemory represents a shared memory segment mapped into the current
// process. Implementations are safe for concurrent use by multiple goroutines
// within one process; inter-process synchronization is the caller's
// responsibility (typically via semaphore).
type SharedMemory interface {
	// Read copies bytes from the segment into p and returns the number
	// copied. Requires len(p) > 0; returns an error otherwise. Returns
	// an error if the segment has been detached.
	Read(p []byte) (int, error)
	// Write copies p into the segment. Returns an error if len(p) exceeds
	// the segment size or if the segment has been detached.
	Write(p []byte) error
	// Clear zeroes the segment (like C's memset(..., 0, ...)). Returns an
	// error if the segment has been detached.
	Clear() error
	// Size returns the segment size in bytes.
	Size() uint
	// Detach unmaps the segment from this process's address space.
	// Idempotent: a second call returns nil.
	Detach() error
	// Remove destroys the underlying kernel object: shm_unlink(3) for
	// POSIX segments, shmctl(IPC_RMID) for System V segments. The kernel
	// defers actual destruction until the last process detaches.
	// Idempotent: a second call returns nil.
	Remove() error
}
