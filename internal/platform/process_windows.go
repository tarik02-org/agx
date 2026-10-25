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

// group holds the job object that kills the command's process tree when it is closed.
type group struct{ job windows.Handle }

func (g *Group) start() error {
	g.cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED}
	if err := g.cmd.Start(); err != nil {
		return err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err == nil {
		g.job = job
		err = assign(job, uint32(g.cmd.Process.Pid))
	}
	if err != nil {
		g.kill()
		_ = g.cmd.Process.Kill()
		_ = g.cmd.Wait()
		return fmt.Errorf("cannot supervise process: %w", err)
	}
	return nil
}

func (g *Group) kill() {
	if g.job != 0 {
		_ = windows.CloseHandle(g.job)
		g.job = 0
	}
}

func (g *Group) terminate() {}

func (g *Group) release() { g.kill() }

// assign puts the suspended process pid in job, then resumes it.
func assign(job windows.Handle, pid uint32) error {
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return err
	}
	// The child cannot spawn descendants before it belongs to the job. x/sys has no
	// NtResumeProcess, so each of its threads is resumed.
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
