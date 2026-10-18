//go:build windows

package hosting

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// The ACL/DPAPI/job primitives follow One Workspace's Apache-2.0 platform
// contract. Project tokens add low integrity, restricting SIDs and job limits.
func checkVolume(dir string) error {
	abs, e := filepath.Abs(dir)
	if e != nil {
		return e
	}
	probe := abs
	for {
		info, e := os.Lstat(probe)
		if e == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("Choose a normal local NTFS directory")
			}
			break
		}
		if !os.IsNotExist(e) {
			return e
		}
		next := filepath.Dir(probe)
		if next == probe {
			return e
		}
		probe = next
	}
	p, _ := windows.UTF16PtrFromString(probe)
	root := make([]uint16, 32768)
	if e = windows.GetVolumePathName(p, &root[0], uint32(len(root))); e != nil {
		return e
	}
	var flags uint32
	if e = windows.GetVolumeInformation(&root[0], nil, 0, nil, nil, &flags, nil, 0); e != nil {
		return e
	}
	if flags&windows.FILE_PERSISTENT_ACLS == 0 || windows.GetDriveType(&root[0]) == windows.DRIVE_REMOTE {
		return errors.New("Private hosting data and project builds need a local NTFS folder with persistent Windows permissions. E:/I: exFAT can hold public downloads, but not protected hosting data")
	}
	return nil
}
func setDACL(path, sddl string) error {
	sd, e := windows.SecurityDescriptorFromString(sddl)
	if e != nil {
		return e
	}
	acl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func privateACL() string {
	u, e := user.Current()
	if e != nil {
		panic(e)
	}
	return "D:P(D;OICI;WDWO;;;RC)(A;OICI;FA;;;" + u.Uid + ")(A;OICI;FA;;;SY)"
}
func prepareData(root string) error {
	if !filepath.IsAbs(root) {
		return errors.New("Use an absolute private-data directory")
	}
	if e := checkVolume(root); e != nil {
		return e
	}
	for p := filepath.Clean(root); ; p = filepath.Dir(p) {
		if f, e := os.Lstat(p); e == nil && f.Mode()&os.ModeSymlink != 0 {
			return errors.New("Private paths must not contain junctions or symbolic links")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	if e := setDACL(root, privateACL()); e != nil {
		return e
	}
	if e := initDirs(root); e != nil {
		return e
	}
	for _, name := range []string{"data", "logs", "cache", "secrets", "caddy", "runtimes", "backups"} {
		if e := filepath.WalkDir(filepath.Join(root, name), func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return errors.New("Protected data contains a link; inspect it before starting")
			}
			return setDACL(p, privateACL())
		}); e != nil {
			return e
		}
	}
	return nil
}
func protect(b []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(b))}
	if len(b) > 0 {
		in.Data = &b[0]
	}
	var out windows.DataBlob
	if e := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN|windows.CRYPTPROTECT_LOCAL_MACHINE, &out); e != nil {
		return nil, e
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
func unprotect(b []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(b))}
	if len(b) > 0 {
		in.Data = &b[0]
	}
	var out windows.DataBlob
	if e := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); e != nil {
		return nil, e
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
func atomicFile(path string, b []byte) error {
	tmp := path + "." + id() + ".tmp"
	if e := os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	defer os.Remove(tmp)
	p, _ := windows.UTF16PtrFromString(tmp)
	q, _ := windows.UTF16PtrFromString(path)
	return windows.MoveFileEx(p, q, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func lockFile(path string) (*os.File, error) {
	p, _ := windows.UTF16PtrFromString(path)
	h, e := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		return nil, errors.New("My Hosting is already running or this data folder is busy")
	}
	return os.NewFile(uintptr(h), path), nil
}
func projectSID(pid string) string {
	h := digest([]byte("OneWorkspace.MyHosting." + pid))
	var a, b, c uint32
	fmt.Sscanf(h[:24], "%08x%08x%08x", &a, &b, &c)
	return fmt.Sprintf("S-1-5-21-%d-%d-%d-1001", a, b, c)
}
func sandboxDir(path, pid string) error {
	if e := checkVolume(path); e != nil {
		return e
	}
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	acl := privateACL() + "(A;OICI;FA;;;" + projectSID(pid) + ")"
	if e := setDACL(path, acl); e != nil {
		return e
	}
	sd, e := windows.SecurityDescriptorFromString("S:(ML;OICI;NW;;;LW)")
	if e != nil {
		return e
	}
	sacl, _, e := sd.SACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl)
}
func allowRuntime(path, pid string) error {
	sd, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	old, _, e := sd.DACL()
	if e != nil {
		return e
	}
	sid, e := windows.StringToSid(projectSID(pid))
	if e != nil {
		return e
	}
	entries := []windows.EXPLICIT_ACCESS{{AccessPermissions: windows.GENERIC_READ | windows.GENERIC_EXECUTE, AccessMode: windows.GRANT_ACCESS, Inheritance: windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_UNKNOWN, TrusteeValue: windows.TrusteeValueFromSID(sid)}}}
	acl, e := windows.ACLFromEntries(entries, old)
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

type ProcessJob struct {
	handle windows.Handle
	cmd    *exec.Cmd
	once   sync.Once
}

func (j *ProcessJob) Close() {
	if j == nil {
		return
	}
	j.once.Do(func() {
		if j.handle != 0 {
			_ = windows.TerminateJobObject(j.handle, 1)
			until := time.Now().Add(5 * time.Second)
			for time.Now().Before(until) {
				var info struct {
					User, Kernel, PeriodUser, PeriodKernel int64
					Faults, Processes, Active, Terminated  uint32
				}
				if windows.QueryInformationJobObject(j.handle, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil) != nil || info.Active == 0 {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			windows.CloseHandle(j.handle)
			j.handle = 0
		}
	})
}
func freezeArtifact(root, pid string) error {
	acl := privateACL() + "(A;OICI;GRGX;;;" + projectSID(pid) + ")"
	sd, e := windows.SecurityDescriptorFromString("S:(ML;OICI;NW;;;ME)")
	if e != nil {
		return e
	}
	label, _, e := sd.SACL()
	if e != nil {
		return e
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("Artifact contains a link")
		}
		if e = setDACL(p, acl); e != nil {
			return e
		}
		return windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, label)
	})
}
func startProcess(cmd *exec.Cmd, pid string, memoryMB int, restricted bool) (*ProcessJob, error) {
	h, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, e
	}
	j := &ProcessJob{handle: h, cmd: cmd}
	fail := func(err error) (*ProcessJob, error) {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		j.Close()
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	info.BasicLimitInformation.ActiveProcessLimit = 32
	if memoryMB > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY
		info.JobMemoryLimit = uintptr(memoryMB) * 1024 * 1024
	}
	if _, e = windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); e != nil {
		return fail(e)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW}
	var token windows.Token
	if restricted {
		current := windows.GetCurrentProcessToken()
		who, e := current.GetTokenUser()
		if e != nil {
			return fail(e)
		}
		if who.User.Sid.String() == "S-1-5-18" {
			return fail(errors.New("Builds cannot run as LocalSystem. Run Hosting with a non-system service account"))
		}
		project, e := windows.StringToSid(projectSID(pid))
		if e != nil {
			return fail(e)
		}
		rc, _ := windows.StringToSid("S-1-5-12")
		users, _ := windows.StringToSid("S-1-5-32-545")
		admin, _ := windows.StringToSid("S-1-5-32-544")
		restrict := []windows.SIDAndAttributes{{Sid: project}, {Sid: rc}, {Sid: users}}
		// Windows process initialization opens the session's window station.
		// Keep its logon SID for that object, while private data ACLs grant only
		// the account/project SIDs. Low integrity and job UI restrictions remain.
		groups, e := current.GetTokenGroups()
		if e != nil { return fail(e) }
		for _, g := range groups.AllGroups() {
			if g.Attributes & windows.SE_GROUP_LOGON_ID == windows.SE_GROUP_LOGON_ID {
				restrict = append(restrict, windows.SIDAndAttributes{Sid:g.Sid})
			}
		}
		disabled := windows.SIDAndAttributes{Sid: admin}
		var original windows.Token
		if e = windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_ADJUST_DEFAULT, &original); e != nil {
			return fail(e)
		}
		defer original.Close()
		proc := windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken")
		ok, _, callErr := proc.Call(uintptr(original), 1, 1, uintptr(unsafe.Pointer(&disabled)), 0, 0, uintptr(len(restrict)), uintptr(unsafe.Pointer(&restrict[0])), uintptr(unsafe.Pointer(&token)))
		if ok == 0 {
			return fail(callErr)
		}
		defer token.Close()
		low, _ := windows.StringToSid("S-1-16-4096")
		label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: low, Attributes: windows.SE_GROUP_INTEGRITY}}
		if e = windows.SetTokenInformation(token, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&label)), uint32(unsafe.Sizeof(label))+uint32(low.Len())); e != nil {
			return fail(e)
		}
		cmd.SysProcAttr.Token = syscall.Token(token)
		ui := uint32(0xff)
		if _, e = windows.SetInformationJobObject(h, windows.JobObjectBasicUIRestrictions, uintptr(unsafe.Pointer(&ui)), 4); e != nil {
			return fail(e)
		}
		cpu := struct{ Flags, Rate uint32 }{5, 2500}
		if _, e = windows.SetInformationJobObject(h, 15, uintptr(unsafe.Pointer(&cpu)), 8); e != nil {
			return fail(e)
		}
	}
	if e = cmd.Start(); e != nil {
		return fail(fmt.Errorf("Start managed process: %w", e))
	}
	ph, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if e != nil {
		return fail(e)
	}
	e = windows.AssignProcessToJobObject(h, ph)
	windows.CloseHandle(ph)
	if e != nil {
		return fail(e)
	}
	snap, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if e != nil {
		return fail(e)
	}
	defer windows.CloseHandle(snap)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for e = windows.Thread32First(snap, &entry); e == nil; e = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID == uint32(cmd.Process.Pid) {
			th, e := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if e != nil {
				return fail(e)
			}
			_, e = windows.ResumeThread(th)
			windows.CloseHandle(th)
			if e != nil {
				return fail(e)
			}
			return j, nil
		}
	}
	return fail(errors.New("Could not resume managed process"))
}
func openBrowser(raw string) {
	c := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", raw)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = c.Start()
}
func safeEnvironment(root, node string, port int, vars map[string]string) []string {
	system := os.Getenv("SystemRoot")
	out := []string{"SystemRoot=" + system, "WINDIR=" + system, "PATH=" + node + ";" + filepath.Join(system, "System32"), "TEMP=" + filepath.Join(root, "tmp"), "TMP=" + filepath.Join(root, "tmp"), "HOME=" + root, "USERPROFILE=" + root, "APPDATA=" + filepath.Join(root, "tmp"), "LOCALAPPDATA=" + filepath.Join(root, "tmp"), "npm_config_cache=" + filepath.Join(root, ".npm-cache"), "npm_config_update_notifier=false", "NODE_ENV=production", fmt.Sprint("PORT=", port), "HOST=127.0.0.1", "HOSTNAME=127.0.0.1"}
	for k, v := range vars {
		if strings.ContainsAny(k, "=\x00") || strings.ContainsRune(v, 0) {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}
