//go:build unix

package shm

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

type Flag int

// System V IPC flags for shmget.
// https://github.com/torvalds/linux/blob/master/include/uapi/linux/ipc.h
const (
	IpcCreat Flag = 00001000 // create if key is nonexistent
	IpcExcl  Flag = 00002000 // fail if key exists

	// Rdonly is a flag for shmat: attach read-only instead of read-write.
	Rdonly Flag = 010000
)

// Permission bits matching <sys/stat.h>.
const (
	SIrusr = 0400        // read by owner
	SIwusr = 0200        // write by owner
	SIrgrp = SIrusr >> 3 // read by group
	SIwgrp = SIwusr >> 3 // write by group
)

const defaultPerm = 0600

type segmentKind uint8

const (
	kindSysV segmentKind = iota + 1
	kindPosix
)

// SharedMemorySegment is a Unix shared memory segment mapped into the current
// process. It can be backed by System V IPC (shmget/shmat) or by a POSIX
// object in /dev/shm (open/ftruncate/mmap); the POSIX backend is Linux-only.
//
// Operations on the segment are safe for concurrent use within one process;
// coordination between processes is the caller's responsibility.
type SharedMemorySegment struct {
	mu   sync.Mutex
	kind segmentKind

	// SysV only.
	id int

	// POSIX only.
	name string

	size    uint
	data    []byte
	removed atomic.Bool // make Remove idempotent across platforms
}

func mergePerm(flgs Flag, perm int) Flag {
	if perm == 0 {
		perm = defaultPerm
	}
	return flgs | Flag(perm)
}

func checkSize(size uint) error {
	if size == 0 {
		return errors.New("size must be greater than zero")
	}
	if size > math.MaxInt {
		return fmt.Errorf("size %d exceeds math.MaxInt", size)
	}
	return nil
}

// NewSharedMemorySegment creates (or opens) a System V shared-memory segment.
//
//	key        - unique identifier (see ftok(3) for systematic generation).
//	size       - size in bytes.
//	permission - octal mode; defaults to 0600 when zero.
//	flags      - any of IpcCreat, IpcExcl.
//	             See https://github.com/torvalds/linux/blob/master/include/uapi/linux/ipc.h
func NewSharedMemorySegment(key int, size uint, permission int, flags ...Flag) (*SharedMemorySegment, error) {
	if err := checkSize(size); err != nil {
		return nil, err
	}

	var createFlags Flag
	for _, f := range flags {
		createFlags |= f
	}
	createFlags = mergePerm(createFlags, permission)

	id, err := unix.SysvShmGet(key, int(size), int(createFlags)) //nolint:gosec // bounded by checkSize
	if err != nil {
		return nil, os.NewSyscallError("shmget", err)
	}

	data, err := unix.SysvShmAttach(id, 0, 0)
	if err != nil {
		return nil, os.NewSyscallError("shmat", err)
	}

	return &SharedMemorySegment{
		kind: kindSysV,
		id:   id,
		size: size,
		data: data,
	}, nil
}

// AttachToShmSegment attaches to an existing System V shared-memory segment
// by its ID. The ID can be discovered with `ipcs -m --human`.
//
//	shmID       - segment ID returned by shmget.
//	size        - size in bytes (must match the creator's size).
//	attachFlags - optional shmat flags (e.g. Rdonly).
func AttachToShmSegment(shmID int, size uint, attachFlags ...Flag) (*SharedMemorySegment, error) {
	if err := checkSize(size); err != nil {
		return nil, err
	}

	var flgs Flag
	for _, f := range attachFlags {
		flgs |= f
	}

	data, err := unix.SysvShmAttach(shmID, 0, int(flgs))
	if err != nil {
		return nil, os.NewSyscallError("shmat", err)
	}

	return &SharedMemorySegment{
		kind: kindSysV,
		id:   shmID,
		size: size,
		data: data,
	}, nil
}

// ID returns the System V SHMID of this segment and true if the segment is
// SysV-backed (and thus has a meaningful ID for AttachToShmSegment).
// Returns (0, false) for POSIX segments, which have no integer identifier.
func (s *SharedMemorySegment) ID() (int, bool) {
	return s.id, s.kind == kindSysV
}

// Size returns the segment size in bytes.
func (s *SharedMemorySegment) Size() uint {
	return s.size
}

// Write copies p into the segment. Returns an error if p is larger than the
// segment or if the segment has been detached.
func (s *SharedMemorySegment) Write(p []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		return errors.New("segment is detached")
	}
	if len(p) > len(s.data) {
		return fmt.Errorf("write size %d exceeds segment size %d", len(p), len(s.data))
	}
	copy(s.data, p)
	return nil
}

// Clear zeroes the segment. Returns an error if the segment has been
// detached (symmetric with Read/Write).
func (s *SharedMemorySegment) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		return errors.New("segment is detached")
	}
	clear(s.data)
	return nil
}

// Read copies up to len(p) bytes from the segment into p and returns the
// number of bytes copied. Rejects zero-length buffers with an error.
func (s *SharedMemorySegment) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		return 0, errors.New("segment is detached")
	}
	if len(p) == 0 {
		return 0, errors.New("read buffer must have length > 0")
	}
	return copy(p, s.data), nil
}

// Detach unmaps the segment from this process's address space. For POSIX
// segments it calls munmap; for SysV, shmdt. After Detach, further Read,
// Write, or Clear calls return an error. Idempotent: a second Detach
// returns nil.
func (s *SharedMemorySegment) Detach() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		return nil
	}

	switch s.kind {
	case kindPosix:
		if err := unix.Munmap(s.data); err != nil {
			return os.NewSyscallError("munmap", err)
		}
	case kindSysV:
		if err := unix.SysvShmDetach(s.data); err != nil {
			return os.NewSyscallError("shmdt", err)
		}
	default:
		return fmt.Errorf("unknown segment kind: %d", s.kind)
	}

	s.data = nil
	return nil
}

// Remove destroys the underlying kernel object. For POSIX segments it
// unlinks /dev/shm/<name>; for SysV, it marks the segment for removal via
// shmctl(IPC_RMID). The kernel defers actual destruction until the last
// process detaches, so calling Remove before all peers have detached is
// safe — they keep working until they leave.
//
// Idempotent: a second call returns nil without re-invoking the syscall.
func (s *SharedMemorySegment) Remove() error {
	if !s.removed.CompareAndSwap(false, true) {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.kind {
	case kindPosix:
		return removePosixName(s.name)
	case kindSysV:
		if _, err := unix.SysvShmCtl(s.id, unix.IPC_RMID, nil); err != nil {
			return os.NewSyscallError("shmctl", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown segment kind: %d", s.kind)
	}
}
