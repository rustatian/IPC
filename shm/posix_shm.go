package shm

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type Flag int

// https://github.com/torvalds/linux/blob/master/include/uapi/linux/ipc.h
const (
	// IpcCreat resource get request flags */
	IpcCreat Flag = 00001000 /* create if key is nonexistent */
	IpcExcl  Flag = 00002000 /* fail if key exists */
	// IPC_NOWAIT Flag = 00004000 /* return error on wait */

	// SHM_R Permission flag for shmget.  */
	// SHM_R Flag = 0400 /* or S_IRUGO from <linux/stat.h> */
	// SHM_W Flag = 0200 /* or S_IWUGO from <linux/stat.h> */

	// Rdonly Flags for 'shmat'.  */
	Rdonly Flag = 010000 /* attach read-only else read-write */
	// SHM_RND   Flag = 020000 /* round attach address to SHMLBA */

	/* Commands for 'shmctl'.  */
	// SHM_REMAP Flag = 040000  /* take-over region on attach */
	// SHM_EXEC  Flag = 0100000 /* execution access */

	// SHM_LOCK   Flag = 11 /* lock segment (root only) */
	// SHM_UNLOCK Flag = 12 /* unlock segment (root only) */
)

const (
	SIrusr = 0400        /* Read by owner.  */
	SIwusr = 0200        /* Write by owner.  */
	SIrgrp = SIrusr >> 3 /* Read by group.  */
	SIwgrp = SIwusr >> 3 /* Write by group.  */
)

type SharedMemorySegment struct {
	key     int
	size    uint
	flags   Flag
	address uintptr
	data    []byte
}

func NewSharedMemoryPosix(name string, size uint, permission int, flags ...Flag) (*SharedMemorySegment, error) {
	// OR (bitwise) flags
	var flgs Flag
	name = fmt.Sprintf("/dev/shm/%s", name)
	for i := 0; i < len(flags); i++ {
		flgs |= flags[i]
	}

	if permission != 0 {
		flgs |= Flag(permission)
	} else {
		flgs |= 0600 // default permission
	}

	fd, err := unix.Open(name, int(flgs), uint32(permission))
	if err != nil {
		return nil, err
	}

	err = unix.Ftruncate(fd, int64(size)) //nolint:gosec
	if err != nil {
		return nil, err
	}

	file := os.NewFile(uintptr(fd), name)
	buff := make([]byte, 13)
	_, err = file.Read(buff)
	if err != nil {
		return nil, err
	}
	fmt.Println(buff)

	buffW := make([]byte, 13)
	buffW[0] = 1
	buffW[1] = 2
	buffW[2] = 3
	_, err = file.WriteAt(buffW, 0)
	if err != nil {
		return nil, err
	}
	err = file.Sync()
	if err != nil {
		return nil, err
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	// data, err := unix.Mmap(fd, 0, int(size), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)

	// file := os.NewFile(uintptr(fd), "some_file")
	// _, err = file.Write([]byte("foo"))
	// if err != nil {
	// 	return nil, err
	// }
	//
	// data[1] = 1
	// fmt.Println(data)
	//
	// err = unix.Munmap(data)
	// if err != nil {
	// 	return nil, err
	// }

	return nil, nil
}

/*
NewSharedMemorySegment the args are:
key - int, used as uniques identifier for the shared memory segment
size - uint, size in bytes to allocate
permission - int, if passed zero, 0600 will be used by default
flags - IpcCreat, IpcExcl, IPC_NOWAIT. More info can be found here https://github.com/torvalds/linux/blob/master/include/uapi/linux/ipc.h
*/
func NewSharedMemorySegment(key int, size uint, permission int, flags ...Flag) (*SharedMemorySegment, error) {
	// OR (bitwise) flags
	var flgs Flag
	for i := 0; i < len(flags); i++ {
		flgs |= flags[i]
	}

	if permission != 0 {
		flgs |= Flag(permission)
	} else {
		flgs |= 0600 // default permission
	}

	// second arg could be uintptr(0) - auto
	// third arg - size
	// fourth - shmflg (flags)
	id, _, errno := syscall.RawSyscall(syscall.SYS_SHMGET, uintptr(key), uintptr(size), uintptr(flgs))
	if errno != 0 {
		return nil, os.NewSyscallError("SYS_SHMGET", errno)
	}

	shmAddr, _, errno := syscall.RawSyscall(syscall.SYS_SHMAT, id, uintptr(0), uintptr(flgs))
	if errno != 0 {
		return nil, errors.New(errno.Error())
	}

	segment := &SharedMemorySegment{
		key:     key,
		size:    size,
		flags:   flgs,
		address: id,
		data:    make([]byte, int(size)), //nolint:gosec
	}

	// construct slice from memory segment
	// sh := (*reflect.SliceHeader)(unsafe.Pointer(&segment.data))
	// sh.Len = int(size)
	// sh.Cap = int(size)
	// sh.Data = shmAddr
	segment.data = unsafe.Slice((*byte)(unsafe.Pointer(shmAddr)), int(size)) //nolint:gosec
	return segment, nil
}

// AttachToShmSegment used to attach to the existing shared memory segment by shared memory ID. Shared memory ID can be known or you find it
// by typing the following command: ipcs -m --human.
// If there is no such shm segment by shmId, the error will be shown.
func AttachToShmSegment(shmID int, size uint, permission int) (*SharedMemorySegment, error) {
	// OR (bitwise) flags
	var flgs Flag
	flgs = flgs | IpcCreat | IpcExcl

	if permission != 0 {
		flgs |= Flag(permission)
	} else {
		flgs |= 0600 // default permission
	}

	shmAddr, _, errno := syscall.RawSyscall(syscall.SYS_SHMAT, uintptr(shmID), uintptr(0), uintptr(flgs))
	if errno != 0 {
		return nil, errors.New(errno.Error())
	}

	segment := &SharedMemorySegment{
		size:    size,
		flags:   flgs,
		address: uintptr(shmID),
		data:    make([]byte, 0),
	}

	// construct slice from memory segment
	// sh := (*reflect.SliceHeader)(unsafe.Pointer(&segment.data))
	// sh.Len = int(size)
	// sh.Cap = int(size)
	// sh.Data = shmAddr

	segment.data = unsafe.Slice((*byte)(unsafe.Pointer(shmAddr)), int(size)) //nolint:gosec

	return segment, nil
}

// write is not thread safe operation
// should be protected via semaphore
func (s *SharedMemorySegment) Write(data []byte) {
	srcLen := len(data)
	dstLen := len(s.data)

	if srcLen > dstLen {
		panic("can't write more than source len")
	}

	s.writeBuffer(data, s.data)
}

// src -> dst
func (s *SharedMemorySegment) writeBuffer(src []byte, dst []byte) {
	copy(dst, src)
}

// Clear by behavior is similar to the std::memset(..., 0, ...)
func (s *SharedMemorySegment) Clear() {
	for i := 0; i < len(s.data); i++ {
		s.data[i] = 0
	}
}

// Read data segment. Attention, the segment to read will be equal to data function arg len
func (s *SharedMemorySegment) Read(data []byte) error {
	if len(data) == 0 {
		return errors.New("allocate []byte with provided length")
	}
	for i := 0; i < len(data); i++ {
		data[i] = s.data[i]
	}
	return nil
}

// Detach used to detach from memory segment
func (s *SharedMemorySegment) Detach() error {
	data := unsafe.SliceData(s.data)
	_, _, errno := syscall.Syscall(syscall.SYS_SHMDT, uintptr(unsafe.Pointer(data)), 0, 0)
	if errno != 0 {
		return errors.New(errno.Error())
	}
	return nil
}
