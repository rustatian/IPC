package semaphore

// IPC_RMID:
// Immediately remove the semaphore set and its associated semid_ds data
// structure. Any processes blocked in semop() calls waiting on semaphores in
// this set are immediately awakened, with semop() reporting the error EIDRM .
// The arg argument is not required.
//---------------
// IPC_STAT
// Place a copy of the semid_ds data structure associated with this semaphore
// set in the buffer pointed to by arg.buf. We describe the semid_ds structure
// in Section 47.4.
//---------------
// IPC_SET
// Update selected fields of the semid_ds data structure associate
