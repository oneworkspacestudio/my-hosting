//go:build windows

package hosting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows"
	"os/user"
	"runtime"
	"unsafe"
)

// Shared with One Workspace's Vault publisher: all local writers of one remote
// tunnel serialize the complete read/merge/write operation. The release must be
// called on the same goroutine because Windows mutex ownership is thread-bound.
func acquireTunnelLock(ctx context.Context, account, tunnel string) (func(), error) {
	runtime.LockOSThread()
	fail := func(e error) (func(), error) { runtime.UnlockOSThread(); return nil, e }
	u, e := user.Current()
	if e != nil {
		return fail(e)
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + u.Uid + ")(A;;GA;;;SY)")
	if e != nil {
		return fail(e)
	}
	hash := sha256.Sum256([]byte(account + "/" + tunnel))
	name, _ := windows.UTF16PtrFromString(`Global\OneWorkspace.Cloudflare.` + hex.EncodeToString(hash[:]))
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, e := windows.CreateMutexEx(&sa, name, 0, windows.SYNCHRONIZE|windows.MUTEX_MODIFY_STATE)
	if h == 0 || (e != nil && e != windows.ERROR_ALREADY_EXISTS) {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return fail(fmt.Errorf("Lock shared Cloudflare tunnel configuration: %v", e))
	}
	for {
		if e = ctx.Err(); e != nil {
			windows.CloseHandle(h)
			return fail(e)
		}
		state, waitErr := windows.WaitForSingleObject(h, 200)
		if waitErr != nil {
			windows.CloseHandle(h)
			return fail(waitErr)
		}
		if state == windows.WAIT_OBJECT_0 || state == windows.WAIT_ABANDONED {
			return func() { _ = windows.ReleaseMutex(h); windows.CloseHandle(h); runtime.UnlockOSThread() }, nil
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			windows.CloseHandle(h)
			return fail(fmt.Errorf("Unexpected tunnel lock status: %d", state))
		}
	}
}
