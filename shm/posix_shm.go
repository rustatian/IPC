package shm

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

type Flags int

// https://github.com/torvalds/linux/blob/master/include/uapi/linux/ipc.h
const (
	/* resource get request flags */
	IPC_CREAT  Flags = 00001000 /* create if key is nonexistent */
	IPC_EXCL   Flags = 00002000 /* fail if key exists */
	IPC_NOWAIT Flags = 00004000 /* return error on wait */
)

type SharedMemorySegment struct {
	path    *byte
	size    uint
	flags   Flags
	address uintptr
	data    []byte
}

// key
// size
func NewSharedMemorySegment(key string, size uint, flags ...Flags) (*SharedMemorySegment, error) {
	path, err := syscall.BytePtrFromString(key)
	if err != nil {
		return nil, ErrCStringCreation
	}

	// OR flags
	var flgs Flags
	for i := 0; i < len(flags); i++ {
		flgs = flgs | flags[i]
	}

	flgs = flgs | 0666

	// second arg could be uintptr(0) - auto
	// third arg - size
	// fourth - shmflg (flags)
	id, _, errno := syscall.RawSyscall(syscall.SYS_SHMGET, uintptr(unsafe.Pointer(path)), uintptr(size), uintptr(flgs))
	if errno != 0 {
		return nil, os.NewSyscallError("SYS_SHMGET", errno)
	}

	shmAddr, _, errno := syscall.RawSyscall(syscall.SYS_SHMAT, id, 0, 0)
	if errno != 0 {
		return nil, errors.New(errno.Error())
	}

	segment := &SharedMemorySegment{
		path:    path,
		size:    size,
		flags:   flgs,
		address: id,
		data:    make([]byte, 0, 0),
	}

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

func (s *SharedMemorySegment) Read() []byte {
	return s.data
}
