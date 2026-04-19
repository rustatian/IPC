//go:build unix

package shm

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

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

const (
	defaultPerm = 0600
	PosixShmDir = "/dev/shm"
)

type segmentKind uint8

const (
	kindSysV segmentKind = iota + 1
	kindPosix
)

// SharedMemorySegment is a Unix shared memory segment mapped into the current
// process. It can be backed by System V IPC (shmget/shmat) or by a POSIX
// object in /dev/shm (open/ftruncate/mmap), distinguished internally.
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

	size uint
	data []byte
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

// NewSharedMemoryPosix creates (or opens) a POSIX shared-memory object by
// name, backed by /dev/shm, and maps it into the process's address space.
// On Linux this is equivalent to shm_open(3) + mmap(2).
//
// The returned segment must be detached with Detach and, when no longer
// needed by any process, removed with Remove (shm_unlink).
func NewSharedMemoryPosix(name string, size uint, permission int) (*SharedMemorySegment, error) {
	if err := checkSize(size); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, errors.New("name must not be empty")
	}
	if permission == 0 {
		permission = defaultPerm
	}

	path := filepath.Join(PosixShmDir, name)

	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR, uint32(permission))
	if err != nil {
		return nil, os.NewSyscallError("open", err)
	}

	if err := unix.Ftruncate(fd, int64(size)); err != nil { //nolint:gosec // bounded by checkSize
		_ = unix.Close(fd)
		return nil, os.NewSyscallError("ftruncate", err)
	}

	data, err := unix.Mmap(fd, 0, int(size), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED) //nolint:gosec // bounded by checkSize
	if err != nil {
		_ = unix.Close(fd)
		return nil, os.NewSyscallError("mmap", err)
	}

	// Descriptor can be released; the mapping keeps the file alive.
	if err := unix.Close(fd); err != nil {
		_ = unix.Munmap(data)
		return nil, os.NewSyscallError("close", err)
	}

	return &SharedMemorySegment{
		kind: kindPosix,
		name: name,
		size: size,
		data: data,
	}, nil
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

// ID returns the System V SHMID of this segment, suitable for passing to
// AttachToShmSegment in another process. Returns 0 for POSIX segments.
func (s *SharedMemorySegment) ID() int {
	return s.id
}

// Size returns the segment size in bytes.
func (s *SharedMemorySegment) Size() uint {
	return s.size
}

// Write copies p into the segment. Returns an error if p is larger than the
// segment.
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

// Clear zeroes the segment (like C's memset(..., 0, ...)).
func (s *SharedMemorySegment) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.data)
}

// Read copies up to len(p) bytes from the segment into p.
// Returns the number of bytes copied.
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
// Write, or Clear calls return an error.
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
func (s *SharedMemorySegment) Remove() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.kind {
	case kindPosix:
		if err := unix.Unlink(filepath.Join(PosixShmDir, s.name)); err != nil {
			return os.NewSyscallError("unlink", err)
		}
	case kindSysV:
		if _, err := unix.SysvShmCtl(s.id, unix.IPC_RMID, nil); err != nil {
			return os.NewSyscallError("shmctl", err)
		}
	default:
		return fmt.Errorf("unknown segment kind: %d", s.kind)
	}
	return nil
}
