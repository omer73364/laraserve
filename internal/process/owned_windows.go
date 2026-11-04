package process

import (
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var ownership struct {
	sync.Once
	job windows.Handle
	err error
}

// Keep the non-inheritable job handle open for this process's entire lifetime.
// Windows closes it on a crash and terminates inherited child job members.
func InitOwnership() error {
	ownership.Do(func() {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			ownership.err = err
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(job)
			ownership.err = err
			return
		}
		if err = windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
			windows.CloseHandle(job)
			ownership.err = err
			return
		}
		ownership.job = job
	})
	return ownership.err
}
func StartOwned(cmd *exec.Cmd) error { return cmd.Start() }
