//go:build linux

package shm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// PosixShmDir is the tmpfs mount where Linux exposes POSIX shared-memory
// objects. glibc's shm_open(3) creates files here with the "/" stripped
// from the name; this package follows the same convention.
const PosixShmDir = "/dev/shm"

// validatePosixName enforces the shm_open(3) name contract: starts with
// "/", contains no other "/", no "..", no NUL byte, and is non-empty
// after the leading slash. Rejecting these prevents path traversal out
// of PosixShmDir.
func validatePosixName(name string) error {
	if name == "" {
		return errors.New("name must not be empty")
	}
	if name[0] != '/' {
		return fmt.Errorf("name %q must start with '/'", name)
	}
	if len(name) < 2 {
		return errors.New("name must have at least one character after '/'")
	}
	rest := name[1:]
	if strings.ContainsAny(rest, "/\x00") {
		return fmt.Errorf("name %q must not contain '/' or NUL after the leading '/'", name)
	}
	if rest == ".." || strings.Contains(rest, "..") {
		return fmt.Errorf("name %q must not contain '..'", name)
	}
	return nil
}

// NewSharedMemoryPosix creates (or opens) a POSIX shared-memory object by
// name, backed by /dev/shm, and maps it into the process's address space.
// This is equivalent to glibc's shm_open(3) + mmap(2).
//
// Linux-only: darwin exposes POSIX named shared memory through distinct
// syscalls (SYS_SHM_OPEN) which this package doesn't implement yet. Use
// NewSharedMemorySegment for portable SysV-backed segments.
//
// The name follows shm_open(3) conventions: must start with "/" and
// contain no other slashes or "..". Permission defaults to 0600.
//
// The returned segment must be detached with Detach and, when no longer
// needed by any process, removed with Remove (which calls shm_unlink).
func NewSharedMemoryPosix(name string, size uint, permission int) (*SharedMemorySegment, error) {
	if err := checkSize(size); err != nil {
		return nil, err
	}
	if err := validatePosixName(name); err != nil {
		return nil, err
	}
	if permission == 0 {
		permission = defaultPerm
	}

	// Strip the leading "/" when mapping to the tmpfs path, matching
	// glibc's shm_open layout: shm_open("/foo", ...) → /dev/shm/foo.
	path := filepath.Join(PosixShmDir, name[1:])

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

// removePosixName unlinks /dev/shm/<name-without-leading-slash>.
func removePosixName(name string) error {
	// Mirror the same stripping as NewSharedMemoryPosix so Remove unlinks
	// the file NewSharedMemoryPosix created.
	path := filepath.Join(PosixShmDir, name[1:])
	if err := unix.Unlink(path); err != nil {
		return os.NewSyscallError("unlink", err)
	}
	return nil
}
