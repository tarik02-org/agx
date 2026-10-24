package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Isolate gives the process its own hidden console so closing the launching terminal does not end it;
// unlike DETACHED_PROCESS, console children it spawns share that console instead of opening windows.
func Isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}

// StartDetached starts the daemon command that build makes outside the job object of its caller,
// with a hidden console of its own. Node-based parents, such as editors and agent harnesses, keep
// their children in a job that kills them when the parent exits, which would kill a daemon an agx
// command in such a process tree started, before it saved its launches. Where the job forbids
// breaking away, a second command starts inside it, as before.
func StartDetached(build func() *exec.Cmd) (*exec.Cmd, error) {
	cmd := build()
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW | windows.CREATE_BREAKAWAY_FROM_JOB,
	}
	err := cmd.Start()
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return cmd, err
	}
	cmd = build()
	Isolate(cmd)
	return cmd, cmd.Start()
}

func IsolateOwned(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED}
}

func Own(cmd *exec.Cmd) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	// The child cannot spawn descendants before it belongs to the job.
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(cmd.Process.Pid) {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			_ = windows.CloseHandle(job)
			return nil, err
		}
		_, err = windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if err != nil {
			_ = windows.CloseHandle(job)
			return nil, err
		}
		resumed = true
	}
	if !resumed {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("cannot resume owned upstream process")
	}
	return func() { _ = windows.CloseHandle(job) }, nil
}

// SurviveBrokenPipes does nothing: Windows has no SIGPIPE, and writes to a closed pipe fail.
func SurviveBrokenPipes() {}

func Interrupts() []os.Signal { return []os.Signal{os.Interrupt} }

// Terminate does nothing: Windows has no termination request a console-less child could handle,
// so callers kill it, with its job, once their grace period ends.
func Terminate(cmd *exec.Cmd) {}

// StopSignals are the signals that stop the daemon gracefully: Ctrl-C and Ctrl-Break, and the
// console close, logoff, and shutdown events, which Go delivers as SIGTERM.
func StopSignals(attached bool) []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }

// TerminalDelivered reports true for console Ctrl-C and Ctrl-Break: the console delivers them to every
// attached process, and Process.Signal cannot send os.Interrupt on Windows.
func TerminalDelivered(pid int, s os.Signal) bool { return s == os.Interrupt }

func Signal(cmd *exec.Cmd, s os.Signal) {
	_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}
