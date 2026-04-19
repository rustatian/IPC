//go:build linux

package semaphore

import (
	"errors"
	"hash/fnv"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// SysV IPC flag/command values used internally. These are defined by the
// Linux kernel ABI; see <linux/ipc.h> and <linux/sem.h>.
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
// The user-facing name is hashed to a 31-bit integer key for semget. The
// 32-bit hash space is large enough that collisions on typical workloads
// are vanishingly rare, but processes that want to share a semaphore MUST
// use the same name — different names may hash to the same key only by
// pathological coincidence.
type Semaphore struct {
	name     string
	semid    uintptr
	unlinked atomic.Bool
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
	return int(h.Sum32() & 0x7FFFFFFF)
}

// NewSemaphore creates a new-named semaphore with the given initial value.
// Returns syscall.EEXIST if a semaphore with this name already exists.
func NewSemaphore(name string, permission int, initial uint) (*Semaphore, error) {
	return open(name, permission, initial, true, true)
}

// OpenSemaphore attaches to an existing named semaphore. Returns
// syscall.ENOENT if none exists.
func OpenSemaphore(name string) (*Semaphore, error) {
	return open(name, 0, 0, false, false)
}

// OpenOrCreateSemaphore creates if absent, opens if present. initial only
// takes effect when the semaphore is newly created.
func OpenOrCreateSemaphore(name string, permission int, initial uint) (*Semaphore, error) {
	return open(name, permission, initial, true, false)
}

func open(name string, permission int, initial uint, create, exclusive bool) (*Semaphore, error) {
	normalized, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	if permission == 0 {
		permission = defaultPerm
	}

	flags := uintptr(permission)
	if create {
		flags |= ipcCreat
	}
	if exclusive {
		flags |= ipcExcl
	}

	key := nameToKey(normalized)
	semid, _, errno := syscall.Syscall(syscall.SYS_SEMGET, uintptr(uint(key)), 1, flags) //nolint:gosec
	if errno != 0 {
		return nil, os.NewSyscallError("semget", errno)
	}

	// If we just created exclusively, initialize the counter.
	if exclusive {
		_, _, errno = syscall.Syscall6(syscall.SYS_SEMCTL, semid, 0, setval, uintptr(initial), 0, 0)
		if errno != 0 {
			// Best-effort cleanup of the set we just created.
			_, _, _ = syscall.Syscall6(syscall.SYS_SEMCTL, semid, 0, ipcRmid, 0, 0, 0)
			return nil, os.NewSyscallError("semctl(SETVAL)", errno)
		}
	}

	return &Semaphore{name: normalized, semid: semid}, nil
}

// Name returns the slash-prefixed semaphore name.
func (s *Semaphore) Name() string { return s.name }

// Post increments the counter, releasing one waiter.
func (s *Semaphore) Post() error {
	return s.semop(&sembuf{semOp: 1})
}

// Wait blocks until the counter is positive, then decrements it.
// Returns syscall.EINTR if interrupted by a signal.
func (s *Semaphore) Wait() error {
	return s.semop(&sembuf{semOp: -1})
}

// TryWait decrements the counter if positive; otherwise returns
// syscall.EAGAIN without blocking.
func (s *Semaphore) TryWait() error {
	return s.semop(&sembuf{semOp: -1, semFlg: semNoWait})
}

func (s *Semaphore) semop(op *sembuf) error {
	_, _, errno := syscall.Syscall(syscall.SYS_SEMOP, s.semid, uintptr(unsafe.Pointer(op)), 1)
	if errno != 0 {
		return os.NewSyscallError("semop", errno)
	}
	return nil
}

// Close releases this process's handle. On Linux SysV semaphore sets have
// no per-process open handle, so this is a no-op — use Unlink to destroy
// the kernel object.
func (s *Semaphore) Close() error { return nil }

// Unlink destroys the semaphore set. Any process blocked in Wait or
// TryWait is awakened with syscall.EIDRM.
func (s *Semaphore) Unlink() error {
	if !s.unlinked.CompareAndSwap(false, true) {
		return nil
	}
	_, _, errno := syscall.Syscall6(syscall.SYS_SEMCTL, s.semid, 0, ipcRmid, 0, 0, 0)
	if errno != 0 {
		return os.NewSyscallError("semctl(IPC_RMID)", errno)
	}
	return nil
}
