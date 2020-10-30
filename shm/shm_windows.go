// +build windows

package shm

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

type SharedMemorySegment struct {
	key  *uint16
	size uint

	handler *syscall.Handle
	addr    uintptr
	data    []byte
}

// https://docs.microsoft.com/en-us/windows/win32/memory/creating-named-shared-memory
func NewSharedMemory(key string, size uint) (*SharedMemorySegment, error) {
	name, err := syscall.UTF16PtrFromString(key)
	if err != nil {
		return nil, err
	}

	hMapFile, err := syscall.CreateFileMapping(syscall.InvalidHandle, nil, syscall.PAGE_READWRITE, 0, uint32(size), name)
	if err != nil {
		return nil, os.NewSyscallError("CreateFileMapping", err)
	}

	pBuf, err := syscall.MapViewOfFile(hMapFile, syscall.FILE_MAP_READ|syscall.FILE_MAP_WRITE, 0, 0, uintptr(size))
	if err != nil {
		return nil, os.NewSyscallError("MapViewOfFile", err)
	}

	segment := &SharedMemorySegment{
		key:     name,
		size:    size,
		handler: &hMapFile,
		data:    make([]byte, int(size), int(size)),
	}

	// construct slice from memory segment
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&pBuf))
	sh.Len = int(size)
	sh.Cap = int(size)
	data := *(*[]byte)(unsafe.Pointer(&sh))
	segment.data = data

	return segment, nil
}

func (shm *SharedMemorySegment) Write(data []byte) {
	for i := 0; i < len(data); i++ {
		shm.data[i] = data[i]
	}
}

// Clear by behaviour is similar to the std::memset(..., 0, ...)
func (shm *SharedMemorySegment) Clear() {
	for i := 0; i < len(shm.data); i++ {
		shm.data[i] = 0
	}
}

// Read data segment. Attention, the segment to read will be equal to data function arg len
func (shm *SharedMemorySegment) Read(data []byte) error {
	if len(data) == 0 {
		return errors.New("allocate []byte with provided length")
	}
	for i := 0; i < len(data); i++ {
		data[i] = shm.data[i]
	}
	return nil
}

// Detach used to detach from memory segment
func (shm *SharedMemorySegment) Detach() error {
	err := syscall.UnmapViewOfFile(shm.addr)
	if err != nil {
		return os.NewSyscallError("UnmapViewOfFile", err)
	}

	err = syscall.CloseHandle(*shm.handler)
	if err != nil {
		return os.NewSyscallError("CloseHandle", err)
	}
	return nil
}
