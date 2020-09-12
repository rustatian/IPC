package semaphore

const (
	IPC_CREAT  Flag = 01000 /* Create key if key does not exist. */
	IPC_EXCL   Flag = 02000 /* Fail if key exists.  */
	IPC_NOWAIT Flag = 04000

	IPC_RMID Flag = 0 /* Remove identifier.  */
	IPC_SET  Flag = 1 /* Set `ipc_perm' options.  */
	IPC_STAT Flag = 2 /* Get `ipc_perm' options.  */
)

const (
	/* Commands for `semctl'.  */
	GETPID  Flag = 11 /* get sempid */
	GETVAL  Flag = 12 /* get semval */
	GETALL  Flag = 13 /* get all semval's */
	GETNCNT Flag = 14 /* get semncnt */
	GETZCNT Flag = 15 /* get semzcnt */
	SETVAL  Flag = 16 /* set semval */
	SETALL  Flag = 17 /* set all semval's */
)