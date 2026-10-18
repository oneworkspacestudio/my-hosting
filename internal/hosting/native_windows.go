//go:build windows

package hosting

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"os/user"
	"sort"
	"strings"
	"unicode/utf16"
	"unsafe"
)

func (j *ProcessJob) Wait() error {
	if j.waitDone == nil {
		return errors.New("Managed process did not start")
	}
	<-j.waitDone
	return j.waitErr
}
func (j *ProcessJob) PID() int { return int(j.pid) }

// Explicit desktop and handle inheritance are required here: exec.Cmd's
// default desktop cannot be initialized by this low-integrity restricted token.
// All project children share a private, hidden desktop, not the user's desktop.
func nativeStart(cmd *exec.Cmd, j *ProcessJob, token windows.Token, restricted bool, pid string) (result *ProcessJob, err error) {
	var desktop windows.Handle
	user32 := windows.NewLazySystemDLL("user32.dll")
	closeDesktop := func() {
		if desktop != 0 {
			_, _, _ = user32.NewProc("CloseDesktop").Call(uintptr(desktop))
			desktop = 0
		}
	}
	fail := func(e error) (*ProcessJob, error) { closeDesktop(); j.Close(); return nil, e }
	si := windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW
	si.ShowWindow = windows.SW_HIDE
	if restricted {
		u, e := user.Current()
		if e != nil {
			return fail(e)
		}
		name := "MyHosting-" + pid[:12] + "-" + id()[:8]
		sd, e := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + u.Uid + ")(A;;GA;;;" + projectSID(pid) + ")S:(ML;;NW;;;LW)")
		if e != nil {
			return fail(e)
		}
		sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
		namePtr, _ := windows.UTF16PtrFromString(name)
		h, _, callErr := user32.NewProc("CreateDesktopW").Call(uintptr(unsafe.Pointer(namePtr)), 0, 0, 0, 0x01ff, uintptr(unsafe.Pointer(&sa)))
		if h == 0 {
			return fail(fmt.Errorf("Create isolated worker desktop: %w", callErr))
		}
		desktop = windows.Handle(h)
		si.Desktop, e = windows.UTF16PtrFromString("winsta0\\" + name)
		if e != nil {
			return fail(e)
		}
	}
	readPipe, writePipe, e := os.Pipe()
	if e != nil {
		return fail(e)
	}
	defer writePipe.Close()
	null, e := os.Open(os.DevNull)
	if e != nil {
		readPipe.Close()
		return fail(e)
	}
	defer null.Close()
	childOut := windows.Handle(writePipe.Fd())
	childIn := windows.Handle(null.Fd())
	for _, h := range []windows.Handle{childOut, childIn} {
		if e = windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); e != nil {
			readPipe.Close()
			return fail(e)
		}
	}
	si.StdOutput = childOut
	si.StdErr = childOut
	si.StdInput = childIn
	attrs, e := windows.NewProcThreadAttributeList(1)
	if e != nil {
		readPipe.Close()
		return fail(e)
	}
	defer attrs.Delete()
	handles := []windows.Handle{childOut, childIn}
	if e = attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); e != nil {
		readPipe.Close()
		return fail(e)
	}
	si.ProcThreadAttributeList = attrs.List()
	application, e := windows.UTF16PtrFromString(cmd.Path)
	if e != nil {
		readPipe.Close()
		return fail(e)
	}
	command, e := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if e != nil {
		readPipe.Close()
		return fail(e)
	}
	var directory *uint16
	if cmd.Dir != "" {
		directory, e = windows.UTF16PtrFromString(cmd.Dir)
		if e != nil {
			readPipe.Close()
			return fail(e)
		}
	}
	envMap := map[string]string{}
	for _, entry := range cmd.Env {
		k, _, ok := strings.Cut(entry, "=")
		if ok {
			envMap[strings.ToUpper(k)] = entry
		}
	}
	keys := make([]string, 0, len(envMap))
	for k := range envMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := []uint16{}
	for _, k := range keys {
		env = append(env, utf16.Encode([]rune(envMap[k]))...)
		env = append(env, 0)
	}
	env = append(env, 0)
	if len(env) == 1 {
		env = append(env, 0)
	}
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW | windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	var info windows.ProcessInformation
	if restricted {
		e = windows.CreateProcessAsUser(token, application, command, nil, nil, true, flags, &env[0], directory, &si.StartupInfo, &info)
	} else {
		e = windows.CreateProcess(application, command, nil, nil, true, flags, &env[0], directory, &si.StartupInfo, &info)
	}
	if e != nil {
		readPipe.Close()
		return fail(fmt.Errorf("Start managed process: %w", e))
	}
	if e = windows.AssignProcessToJobObject(j.handle, info.Process); e != nil {
		windows.TerminateProcess(info.Process, 1)
		windows.CloseHandle(info.Thread)
		windows.CloseHandle(info.Process)
		readPipe.Close()
		return fail(e)
	}
	if _, e = windows.ResumeThread(info.Thread); e != nil {
		windows.TerminateProcess(info.Process, 1)
		windows.CloseHandle(info.Thread)
		windows.CloseHandle(info.Process)
		readPipe.Close()
		return fail(e)
	}
	windows.CloseHandle(info.Thread)
	j.pid = info.ProcessId
	j.waitDone = make(chan struct{})
	output := cmd.Stdout
	if output == nil {
		output = cmd.Stderr
	}
	if output == nil {
		output = io.Discard
	}
	ioDone := make(chan struct{})
	go func() { _, _ = io.Copy(output, readPipe); readPipe.Close(); close(ioDone) }()
	writePipe.Close()
	go func() {
		defer close(j.waitDone)
		_, e := windows.WaitForSingleObject(info.Process, windows.INFINITE)
		var code uint32
		if e == nil {
			e = windows.GetExitCodeProcess(info.Process, &code)
		}
		windows.CloseHandle(info.Process)
		j.Close()
		<-ioDone
		closeDesktop()
		if e != nil {
			j.waitErr = e
		} else if code != 0 {
			j.waitErr = fmt.Errorf("Managed process exited with code 0x%x", code)
		}
	}()
	return j, nil
}
