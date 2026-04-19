//go:build darwin

package semaphore

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// Darwin POSIX named semaphores. syscall.SYS_SEM_* constants expose the
// kernel traps; these have fixed-argument signatures (the variadic C
// sem_open is a libc convenience, not the kernel interface), so raw
// syscall works without going through libSystem.

const defaultPerm = 0600

// semFailed is the POSIX SEM_FAILED sentinel — (void*)-1. sem_open
// returns this on failure and sets errno.
const semFailed = ^uintptr(0)

// Semaphore is a Darwin-backed named counting semaphore.
type Semaphore struct {
	name     string
	handle   uintptr
	closed   atomic.Bool
	unlinked atomic.Bool
}

func normalizeName(name string) (string, error) {
	if name == "" {
		return "", errors.New("name must not be empty")
	}
	if name[0] != '/' {
		name = "/" + name
	}
	return name, nil
}

// NewSemaphore creates a new named semaphore with the given initial value.
// Returns syscall.EEXIST if a semaphore with this name already exists.
func NewSemaphore(name string, permission int, initial uint) (*Semaphore, error) {
	return open(name, syscall.O_CREAT|syscall.O_EXCL, permission, initial)
}

// OpenSemaphore attaches to an existing named semaphore. Returns
// syscall.ENOENT if none exists.
func OpenSemaphore(name string) (*Semaphore, error) {
	return open(name, 0, 0, 0)
}

// OpenOrCreateSemaphore creates if absent, opens if present. initial only
// takes effect when the semaphore is newly created.
func OpenOrCreateSemaphore(name string, permission int, initial uint) (*Semaphore, error) {
	return open(name, syscall.O_CREAT, permission, initial)
}

func open(name string, oflag int, permission int, initial uint) (*Semaphore, error) {
	normalized, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	if permission == 0 {
		permission = defaultPerm
	}

	cName, err := syscall.ByteSliceFromString(normalized)
	if err != nil {
		return nil, err
	}

	handle, _, errno := syscall.Syscall6(
		syscall.SYS_SEM_OPEN,
		uintptr(unsafe.Pointer(&cName[0])),
		uintptr(oflag), //nolint:gosec // kernel oflag bits are always positive
		uintptr(permission),
		uintptr(initial),
		0, 0,
	)
	if handle == semFailed {
		// In the (unlikely) event errno is zero, surface an explicit
		// error rather than the nonsense "sem_open: no error" message.
		if errno == 0 {
			return nil, fmt.Errorf("sem_open %q: SEM_FAILED returned with errno=0", normalized)
		}
		return nil, os.NewSyscallError("sem_open", errno)
	}
	return &Semaphore{name: normalized, handle: handle}, nil
}

// Name returns the slash-prefixed semaphore name.
func (s *Semaphore) Name() string { return s.name }

// Post increments the counter, releasing one waiter.
func (s *Semaphore) Post() error {
	if s.closed.Load() {
		return errors.New("semaphore is closed")
	}
	_, _, errno := syscall.Syscall(syscall.SYS_SEM_POST, s.handle, 0, 0)
	if errno != 0 {
		return os.NewSyscallError("sem_post", errno)
	}
	return nil
}

// Wait blocks until the counter is positive, then decrements it.
// Returns syscall.EINTR if interrupted by a signal.
func (s *Semaphore) Wait() error {
	if s.closed.Load() {
		return errors.New("semaphore is closed")
	}
	_, _, errno := syscall.Syscall(syscall.SYS_SEM_WAIT, s.handle, 0, 0)
	if errno != 0 {
		return os.NewSyscallError("sem_wait", errno)
	}
	return nil
}

// TryWait decrements the counter if positive; otherwise returns
// syscall.EAGAIN without blocking.
func (s *Semaphore) TryWait() error {
	if s.closed.Load() {
		return errors.New("semaphore is closed")
	}
	_, _, errno := syscall.Syscall(syscall.SYS_SEM_TRYWAIT, s.handle, 0, 0)
	if errno != 0 {
		return os.NewSyscallError("sem_trywait", errno)
	}
	return nil
}

// Close releases this process's handle. Idempotent: a second call returns
// nil without re-invoking the syscall. The kernel object persists until
// Unlink is called (typically by the creator).
func (s *Semaphore) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	_, _, errno := syscall.Syscall(syscall.SYS_SEM_CLOSE, s.handle, 0, 0)
	if errno != 0 {
		return os.NewSyscallError("sem_close", errno)
	}
	return nil
}

// Unlink removes the semaphore's name from the system. Existing handles
// in other processes keep working until they Close; the kernel reclaims
// the object when the last reference drops. Idempotent.
func (s *Semaphore) Unlink() error {
	if !s.unlinked.CompareAndSwap(false, true) {
		return nil
	}
	cName, err := syscall.ByteSliceFromString(s.name)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall(
		syscall.SYS_SEM_UNLINK,
		uintptr(unsafe.Pointer(&cName[0])),
		0, 0,
	)
	if errno != 0 {
		return os.NewSyscallError("sem_unlink", errno)
	}
	return nil
}
