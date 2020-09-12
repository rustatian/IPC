package shm

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

type Flag int

// https://github.com/torvalds/linux/blob/master/include/uapi/linux/ipc.h
const (
	/* resource get request flags */
	IPC_CREAT  Flag = 00001000 /* create if key is nonexistent */
	IPC_EXCL   Flag = 00002000 /* fail if key exists */
	IPC_NOWAIT Flag = 00004000 /* return error on wait */

	/* Permission flag for shmget.  */
	SHM_R Flag = 0400 /* or S_IRUGO from <linux/stat.h> */
	SHM_W Flag = 0200 /* or S_IWUGO from <linux/stat.h> */

	/* Flags for `shmat'.  */
	SHM_RDONLY Flag = 010000 /* attach read-only else read-write */
	SHM_RND    Flag = 020000 /* round attach address to SHMLBA */

	/* Commands for `shmctl'.  */
	SHM_REMAP Flag = 040000  /* take-over region on attach */
	SHM_EXEC  Flag = 0100000 /* execution access */

	SHM_LOCK   Flag = 11 /* lock segment (root only) */
	SHM_UNLOCK Flag = 12 /* unlock segment (root only) */
)

const (
	S_IRUSR = 0400         /* Read by owner.  */
	S_IWUSR = 0200         /* Write by owner.  */
	S_IRGRP = S_IRUSR >> 3 /* Read by group.  */
	S_IWGRP = S_IWUSR >> 3 /* Write by group.  */
)

type SharedMemorySegment struct {
	path    *byte
	size    uint
	flags   Flag
	address uintptr
	data    []byte
}

// key
// size
//(S_IRUSR | S_IWUSR | S_IRGRP | S_IWGRP)
func NewSharedMemorySegment(key string, size uint, permission int, flags ...Flag) (*SharedMemorySegment, error) {
	path, err := syscall.BytePtrFromString(key)
	if err != nil {
		return nil, ErrCStringCreation
	}

	// OR (bitwise) flags
	var flgs Flag
	for i := 0; i < len(flags); i++ {
		flgs = flgs | flags[i]
	}

	if permission != 0 {
		flgs = flgs | Flag(permission)
	} else {
		flgs = flgs | 0600 // default permission
	}

	// second arg could be uintptr(0) - auto
	// third arg - size
	// fourth - shmflg (flags)
	id, _, errno := syscall.RawSyscall(syscall.SYS_SHMGET, uintptr(unsafe.Pointer(path)), uintptr(size), uintptr(flgs))
	if errno != 0 {
		return nil, os.NewSyscallError("SYS_SHMGET", errno)
	}

	shmAddr, _, errno := syscall.RawSyscall(syscall.SYS_SHMAT, id, uintptr(0), uintptr(flgs))
	if errno != 0 {
		return nil, errors.New(errno.Error())
	}

	segment := &SharedMemorySegment{
		path:    path,
		size:    size,
		flags:   flgs,
		address: id,
		//data:    make([]byte, 0, 0),
	}

	// construct slice from memory segment
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&segment.data))
	sh.Len = 0
	sh.Cap = int(size)
	sh.Data = shmAddr

	segment.data = *(*[]byte)(unsafe.Pointer(sh))

	return segment, nil
}

// write is not thread safe operation
// should be protected via semaphore
func (s *SharedMemorySegment) Write(data []byte) {
	s.data = append(s.data, data...)
}

func (s *SharedMemorySegment) Detach() error {
	data := (*reflect.SliceHeader)(unsafe.Pointer(&s.data))
	_, _, errno := syscall.Syscall(syscall.SYS_SHMDT, data.Data, 0, 0)
	if errno != 0 {
		return errors.New(errno.Error())
	}
	return nil
}

func (s *SharedMemorySegment) Read() []byte {
	return s.data
}
