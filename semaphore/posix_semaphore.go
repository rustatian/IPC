// +build linux

package semaphore

import (
	"errors"
	"syscall"
	"unsafe"
)

/* The following System V style IPC functions implement a semaphore
   handling.  The definition is found in XPG2.  */

/* Structure used for argument to `semop' to describe operations.  */
//struct sembuf
//{
//unsigned short int sem_num;	/* semaphore number */
//short int sem_op;		/* semaphore operation */
//short int sem_flg;		/* operation flag */
//};

//The sem_num field identifies the semaphore within the set upon which the opera-
//tion is to be performed. The sem_op field specifies the operation to be performed:
//
//If sem_op is greater than 0, the value of sem_op is added to the semaphore value.
//As a result, other processes waiting to decrease the semaphore value may be
//awakened and perform their operations. The calling process must have alter
//(write) permission on the semaphore.

//If sem_op equals 0, the value of the semaphore is checked to see whether it cur-
//rently equals 0. If it does, the operation completes immediately; otherwise,
//semop() blocks until the semaphore value becomes 0. The calling process must
//have read permission on the semaphore.

//If sem_op is less than 0, decrease the value of the semaphore by the amount
//specified in sem_op. If the current value of the semaphore is greater than or
//equal to the absolute value of sem_op, the operation completes immediately.
//Otherwise, semop() blocks until the semaphore value has been increased to a
//level that permits the operation to be performed without resulting in a nega-
//tive value. The calling process must have alter permission on the semaphore.

type Flag int

type Semaphore struct {
	key    int
	nsems  int
	semflg Flag

	semid uintptr
}

type sembuf struct {
	sem_num uint16 // according to the standard, unsigned short should be at least 2 bytes
	sem_op  int16  // /* Operation to be performed */
	sem_flg int16  // /* Operation flags (IPC_NOWAIT and SEM_UNDO) */
}

type SystemVSemaphore interface {
	GetValue(key int) (int, error)
	Add(semNum int) error
	Done(semNum int) error
	Wait() error
}

func NewSemaphore(key int, nsems int, permission int, reset bool, semflg ...Flag) (*Semaphore, error) {
	// OR flags
	var flgs Flag
	for i := 0; i < len(semflg); i++ {
		flgs = flgs | semflg[i]
	}

	if permission != 0 {
		flgs = flgs | Flag(permission)
	} else {
		flgs = flgs | 0600 // default permission
	}

	semid, _, errno := syscall.Syscall(syscall.SYS_SEMGET, uintptr(key), uintptr(nsems), uintptr(flgs))
	if errno != 0 {
		return nil, errors.New(errno.Error())
	}

	if reset {
		// reset value of the semaphore to the 0
		_, _, errno = syscall.Syscall6(syscall.SYS_SEMCTL, semid, uintptr(0), uintptr(SETVAL), uintptr(0), uintptr(0), uintptr(0))
		if errno != 0 {
			return nil, errors.New(errno.Error())
		}
	}

	return &Semaphore{
		key:    key,
		nsems:  nsems, // number of semaphores in the set
		semflg: flgs,
		semid:  semid,
	}, nil
}

// int semget(key_t key , int nsems , int semflg );
// flags which can be used:
// IPC_CREAT - If no semaphore set with the specified key exists, create a new set.
// IPC_EXCL If IPC_CREAT was also specified, and a semaphore set with the specified key already exists, fail with the error EEXIST
// return semaphore ID
func (s *Semaphore) GetValue(key int) (int, error) {
	semid, _, errno := syscall.Syscall(syscall.SYS_SEMGET, uintptr(key), uintptr(s.nsems), uintptr(s.semflg))
	if errno != 0 {
		return -1, errors.New(errno.Error())
	}

	return int(semid), nil
}

// semNum in most cases is 0, but if you initialized semaphore with nsems more then 1, semNum will be you target semaphore
func (s *Semaphore) Add(semNum int) error {
	sops := &sembuf{
		sem_num: uint16(semNum),
		sem_op:  1,
		sem_flg: 0,
	}
	// the last arg is a len of sops
	_, _, errno := syscall.Syscall(syscall.SYS_SEMOP, s.semid, uintptr(unsafe.Pointer(sops)), uintptr(1))
	if errno != 0 {
		return errors.New(errno.Error())
	}
	return nil
}

func (s *Semaphore) Done(semNum int) error {
	sops := &sembuf{
		sem_num: uint16(semNum),
		sem_op:  -1,
		sem_flg: 0,
	}
	// the last arg is a len of sops
	_, _, errno := syscall.Syscall(syscall.SYS_SEMOP, s.semid, uintptr(unsafe.Pointer(sops)), uintptr(1))
	if errno != 0 {
		return errors.New(errno.Error())
	}
	return nil
}

func (s *Semaphore) Wait() error {
	sops := &sembuf{
		sem_num: 0,  // sem number is the semaphor number in the set. If you declared nsems 1, here should be 0
		sem_op:  0, // operation
		sem_flg: 0,
	}

	// int semop(int semid , struct sembuf * sops , unsigned int nsops );
	// The sops argument is a pointer to an array that contains the operations to be performed, and nsops gives the size of this array
	_, _, errno := syscall.Syscall(syscall.SYS_SEMOP, s.semid, uintptr(unsafe.Pointer(sops)), uintptr(1))
	if errno != 0 {
		return errors.New(errno.Error())
	}
	return nil
}
