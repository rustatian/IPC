//go:build linux

package semaphore

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// SysV IPC flag/command values used internally. Defined by the Linux
// kernel ABI; see <linux/ipc.h> and <linux/sem.h>.
const (
	ipcCreat  = 01000
	ipcExcl   = 02000
	ipcRmid   = 0
	setval    = 16
	semNoWait = 04000 // IPC_NOWAIT as a semop flag

	defaultPerm = 0600
)

// Semaphore is a Linux SysV-backed named counting semaphore.
//
// Hash collision caveat: the user-facing name is FNV-1a-hashed to a 32-bit
// integer key for semget. Two different names hashing to the same key
// would silently share the same kernel semaphore set — a correctness
// hazard with no runtime signal. The 32-bit key space gives a birthday
// bound of ~77k names at 50% collision probability, so practical risk is
// low, but to eliminate it entirely, use unique name prefixes for your
// application (e.g. "/myapp.v1.jobs" rather than "/jobs") so you compete
// only with yourself in the key space.
type Semaphore struct {
	name     string
	semid    uintptr
	closed   atomic.Bool // gate Post/Wait/TryWait after Close for parity with darwin
	unlinked atomic.Bool // make Unlink idempotent
}

type sembuf struct {
	semNum uint16
	semOp  int16
	semFlg int16
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

func nameToKey(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	// Use the full 32-bit key space. SysV keys are signed int; passing a
	// negative key to semget is legal and maximizes the effective range.
	return int(int32(h.Sum32())) //nolint:gosec // kernel accepts any int32 as key
}

// NewSemaphore creates a new named semaphore with the given initial value.
// Returns syscall.EEXIST if a semaphore with this name already exists.
func NewSemaphore(name string, permission int, initial uint) (*Semaphore, error) {
	s, _, err := openOrCreate(name, permission, initial, true)
	return s, err
}

// OpenSemaphore attaches to an existing named semaphore. Returns
// syscall.ENOENT if none exists.
func OpenSemaphore(name string) (*Semaphore, error) {
	normalized, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	key := nameToKey(normalized)
	semid, _, errno := syscall.Syscall(syscall.SYS_SEMGET, uintptr(uint(key)), 1, 0) //nolint:gosec
	if errno != 0 {
		return nil, os.NewSyscallError("semget", errno)
	}
	return &Semaphore{name: normalized, semid: semid}, nil
}

// OpenOrCreateSemaphore creates if absent, opens if present. initial only
// takes effect when the semaphore is newly created.
func OpenOrCreateSemaphore(name string, permission int, initial uint) (*Semaphore, error) {
	s, _, err := openOrCreate(name, permission, initial, false)
	return s, err
}

// openOrCreate implements the common semget-then-setval dance. Returning
// `created` lets callers distinguish "I initialized the counter" from "I
// attached to an existing one". When exclusive is true, EEXIST is
// surfaced; otherwise, it's resolved to an attach.
func openOrCreate(name string, permission int, initial uint, exclusive bool) (_ *Semaphore, created bool, _ error) {
	normalized, err := normalizeName(name)
	if err != nil {
		return nil, false, err
	}
	if permission == 0 {
		permission = defaultPerm
	}

	key := nameToKey(normalized)

	// First attempt: exclusive create. Always try this so we can tell if
	// we were the creator (and thus the one responsible for SETVAL).
	createFlags := uintptr(permission) | ipcCreat | ipcExcl
	semid, _, errno := syscall.Syscall(syscall.SYS_SEMGET, uintptr(uint(key)), 1, createFlags) //nolint:gosec
	switch {
	case errno == 0:
		// We created the set; initialize the counter.
		if err := semctlSetval(semid, initial); err != nil {
			// Best-effort cleanup with both errors preserved.
			if rmErr := semctlRemove(semid); rmErr != nil {
				return nil, false, errors.Join(err, fmt.Errorf("cleanup: %w", rmErr))
			}
			return nil, false, err
		}
		return &Semaphore{name: normalized, semid: semid}, true, nil

	case errors.Is(errno, syscall.EEXIST):
		if exclusive {
			return nil, false, os.NewSyscallError("semget", errno)
		}
		// Fall through to plain attach.
		semid, _, errno = syscall.Syscall(syscall.SYS_SEMGET, uintptr(uint(key)), 1, uintptr(permission)) //nolint:gosec
		if errno != 0 {
			return nil, false, os.NewSyscallError("semget", errno)
		}
		return &Semaphore{name: normalized, semid: semid}, false, nil

	default:
		return nil, false, os.NewSyscallError("semget", errno)
	}
}

func semctlSetval(semid uintptr, val uint) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_SEMCTL, semid, 0, setval, uintptr(val), 0, 0)
	if errno != 0 {
		return os.NewSyscallError("semctl(SETVAL)", errno)
	}
	return nil
}

func semctlRemove(semid uintptr) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_SEMCTL, semid, 0, ipcRmid, 0, 0, 0)
	if errno != 0 {
		return os.NewSyscallError("semctl(IPC_RMID)", errno)
	}
	return nil
}

// Name returns the slash-prefixed semaphore name.
func (s *Semaphore) Name() string { return s.name }

// Post increments the counter, releasing one waiter.
func (s *Semaphore) Post() error {
	if s.closed.Load() {
		return errors.New("semaphore is closed")
	}
	return s.semop(&sembuf{semOp: 1})
}

// Wait blocks until the counter is positive, then decrements it.
// Returns syscall.EINTR if interrupted by a signal.
func (s *Semaphore) Wait() error {
	if s.closed.Load() {
		return errors.New("semaphore is closed")
	}
	return s.semop(&sembuf{semOp: -1})
}

// TryWait decrements the counter if positive; otherwise returns
// syscall.EAGAIN without blocking.
func (s *Semaphore) TryWait() error {
	if s.closed.Load() {
		return errors.New("semaphore is closed")
	}
	return s.semop(&sembuf{semOp: -1, semFlg: semNoWait})
}

func (s *Semaphore) semop(op *sembuf) error {
	_, _, errno := syscall.Syscall(syscall.SYS_SEMOP, s.semid, uintptr(unsafe.Pointer(op)), 1)
	if errno != 0 {
		return os.NewSyscallError("semop", errno)
	}
	return nil
}

// Close marks the semaphore handle as unusable by this process. SysV
// semaphore sets have no per-process kernel handle to release, so this is
// purely a state gate — after Close, Post/Wait/TryWait return an error.
// This gives parity with darwin's Close behavior, so cross-platform
// callers can rely on `defer s.Close()` for identical semantics.
func (s *Semaphore) Close() error {
	s.closed.Store(true)
	return nil
}

// Unlink destroys the semaphore set. Any process blocked in Wait or
// TryWait is awakened with syscall.EIDRM. Idempotent: a second call
// returns nil without re-invoking the syscall.
func (s *Semaphore) Unlink() error {
	if !s.unlinked.CompareAndSwap(false, true) {
		return nil
	}
	return semctlRemove(s.semid)
}
