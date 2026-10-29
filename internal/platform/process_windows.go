package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
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

// group holds the job object that kills the command's process tree when it is closed.
type group struct{ job windows.Handle }

func (g *Group) start() error {
	g.cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	var err error
	g.job, err = startInJob(g.cmd, windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE)
	if err != nil {
		g.kill()
		if g.cmd.Process != nil && g.cmd.ProcessState == nil {
			_ = g.cmd.Process.Kill()
			_ = g.cmd.Wait()
		}
		return fmt.Errorf("cannot supervise process: %w", err)
	}
	return nil
}

func startInJob(cmd *exec.Cmd, flags uint32) (windows.Handle, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &windows.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := uint32(cmd.Process.Pid)
	job, err := windows.CreateJobObject(nil, nil)
	if err == nil {
		if err = assign(job, pid, flags); err != nil {
			_ = windows.CloseHandle(job)
			job = 0
		}
	}
	// A supervised group must never run outside its job; tied harnesses may continue.
	if err != nil && flags&windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK == 0 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return 0, err
	}
	if resumeErr := resume(pid); resumeErr != nil {
		if job != 0 {
			_ = windows.CloseHandle(job)
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return 0, fmt.Errorf("cannot start process: %w", resumeErr)
	}
	return job, err
}

func (g *Group) kill() {
	if g.job != 0 {
		_ = windows.CloseHandle(g.job)
		g.job = 0
	}
}

func (g *Group) terminate() {}

func (g *Group) release() { g.kill() }

// StartTied starts cmd in a job object that ends it and its descendants when this process ends
// before calling untie. A host that stops agx terminates it, which on Windows ends no child, so a
// harness would otherwise keep running cut off from its launch. untie lets the descendants left
// outlive this process, as they would without the job; they may also break away from it. When the
// job cannot be set up, cmd runs untied, and StartTied returns the error with a no-op untie.
func StartTied(cmd *exec.Cmd) (untie func(), err error) {
	job, err := startInJob(cmd, windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE|windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK)
	if cmd.Process == nil || cmd.ProcessState != nil {
		return nil, err
	}
	if job == 0 {
		return func() {}, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = limit(job, windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK)
			_ = windows.CloseHandle(job)
		})
	}, nil
}

// Alive reports whether process pid runs; one whose PID was reused since reads as running.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(process) }()
	event, err := windows.WaitForSingleObject(process, 0)
	return err != nil || event == uint32(windows.WAIT_TIMEOUT)
}

// limit sets job's limit flags.
func limit(job windows.Handle, flags uint32) error {
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = flags
	_, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	return err
}

// assign sets job's limit flags and puts the suspended process pid in it.
func assign(job windows.Handle, pid, flags uint32) error {
	if err := limit(job, flags); err != nil {
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	return windows.AssignProcessToJobObject(job, process)
}

// resume resumes the suspended process pid. Started suspended, it cannot spawn descendants
// before it belongs to a job. x/sys has no NtResumeProcess, so each of its threads is resumed.
func resume(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if err != nil {
			return err
		}
		resumed = true
	}
	if !resumed {
		return errors.New("no thread to resume")
	}
	return nil
}

// SurviveBrokenPipes does nothing: Windows has no SIGPIPE, and writes to a closed pipe fail.
func SurviveBrokenPipes() {}

func Interrupts() []os.Signal { return []os.Signal{os.Interrupt} }

// StopSignals are the signals that stop the daemon gracefully: Ctrl-C and Ctrl-Break, and the
// console close, logoff, and shutdown events, which Go delivers as SIGTERM.
func StopSignals(attached bool) []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }

// TerminalDelivered reports true for console Ctrl-C and Ctrl-Break: the console delivers them to every
// attached process, and Process.Signal cannot send os.Interrupt on Windows.
func TerminalDelivered(pid int, s os.Signal) bool { return s == os.Interrupt }
