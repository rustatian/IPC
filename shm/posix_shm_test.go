//go:build !windows

package shm

import (
	"testing"

	"github.com/rustatian/ipc/shm/test"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

const testData = "hello my dear friend"

func TestNewSharedMemorySegmentPOSIX(t *testing.T) {
	t.Skip("test is not ready")
	shms, err := NewSharedMemoryPosix("foo", 1024 /*unix.S_IRUSR|unix.S_IWUSR*/, unix.O_CREAT, unix.O_RDWR)
	if err != nil {
		panic(err)
	}

	_ = shms
}

func TestNewSharedMemorySegment(t *testing.T) {
	testBuf := make([]byte, 0)
	testBuf = append(testBuf, []byte(testData)...)

	seg1, err := NewSharedMemorySegment(0x1, 1024, SIrusr|SIwusr|SIrgrp|SIwgrp, IpcCreat)
	if err != nil {
		t.Fatal(err)
	}

	// write data to the shared memory
	seg1.Write([]byte(testData))
	err = seg1.Detach()
	if err != nil {
		t.Fatal(err)
	}

	seg2, err := NewSharedMemorySegment(0x1, 1024, 0, Rdonly)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(testData))
	err = seg2.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	err = seg2.Detach()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, testBuf, buf)
}

func TestAttachToShmSegment(t *testing.T) {
	testBuf := make([]byte, 0)
	testBuf = append(testBuf, []byte(testData)...)
	// Just to be sure, that shm segment exists
	seg1, err := NewSharedMemorySegment(0x1, 1024, SIrusr|SIwusr|SIrgrp|SIwgrp, IpcCreat)
	if err != nil {
		t.Fatal(err)
	}

	// clear shm segment
	seg1.Clear()

	// write data to the shared memory
	seg1.Write([]byte(testData))
	err = seg1.Detach()
	if err != nil {
		t.Fatal(err)
	}

	seg2, err := AttachToShmSegment(int(seg1.address), 1024, 0666)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len(testData))
	err = seg2.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	err = seg2.Detach()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, testBuf, buf)
}

// 75 microseconds - Read
func BenchmarkAttachToShmSegment_READ(b *testing.B) {
	bigJSONLen := len(test.BigJSON)
	testBuf := make([]byte, 0, len(testData))
	testBuf = append(testBuf, testData...)
	// Just to be sure, that shm segment exists
	seg1, err := NewSharedMemorySegment(0x10, uint(bigJSONLen), SIrusr|SIwusr|SIrgrp|SIwgrp, IpcCreat) //nolint:gosec
	if err != nil {
		b.Fatal(err)
	}

	// clear shm segment
	seg1.Clear()

	// write data to the shared memory
	seg1.Write(testBuf)
	err = seg1.Detach()
	if err != nil {
		b.Fatal(err)
	}

	seg2, err := AttachToShmSegment(int(seg1.address), uint(bigJSONLen), 0666) //nolint:gosec
	if err != nil {
		b.Fatal(err)
	}

	buf := make([]byte, bigJSONLen)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = seg2.Read(buf)
		if err != nil {
			b.Fatal(err)
		}
	}

	err = seg2.Detach()
	if err != nil {
		b.Fatal(err)
	}
}

// 135 microseconds - Write
// 50880	     23679 ns/op	  147456 B/op	       1 allocs/op
// 10639	    152172 ns/op	  147456 B/op	       1 allocs/op
func BenchmarkAttachToShmSegment_WRITE(b *testing.B) {
	bigJSONLen := len(test.BigJSON)
	testBuf := make([]byte, 0, len(testData))
	testBuf = append(testBuf, testData...)
	// Just to be sure, that shm segment exists
	seg1, err := NewSharedMemorySegment(0x20, uint(bigJSONLen), SIrusr|SIwusr|SIrgrp|SIwgrp, IpcCreat) //nolint:gosec
	if err != nil {
		b.Fatal(err)
	}

	// clear shm segment
	seg1.Clear()

	// write data to the shared memory
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		seg1.Write(testBuf)
		seg1.Clear()
	}

	err = seg1.Detach()
	if err != nil {
		b.Fatal(err)
	}
}
