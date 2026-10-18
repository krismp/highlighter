package desktop

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	ole32                    = syscall.NewLazyDLL("ole32.dll")
	procSHGetKnownFolderPath = shell32.NewProc("SHGetKnownFolderPath")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	procSHFileOperationW     = shell32.NewProc("SHFileOperationW")
	// FOLDERID_Videos {18989B1D-99B5-455B-841C-AB7C74E4DDFC}
	folderIDVideos = syscall.GUID{Data1: 0x18989B1D, Data2: 0x99B5, Data3: 0x455B,
		Data4: [8]byte{0x84, 0x1C, 0xAB, 0x7C, 0x74, 0xE4, 0xDD, 0xFC}}
)

// videosDir asks Windows for the Videos known folder, which users can move
// (OneDrive, another drive) and whose name is localized.
func videosDir(home string) string {
	var p *uint16
	r, _, _ := procSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&folderIDVideos)), 0, 0, uintptr(unsafe.Pointer(&p)))
	if r != 0 || p == nil {
		return filepath.Join(home, "Videos")
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	var path []uint16
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; ptr = unsafe.Add(ptr, 2) {
		path = append(path, *(*uint16)(ptr))
	}
	return syscall.UTF16ToString(path)
}

// Open opens a URL, folder or file with its default app.
func Open(target string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Run()
}

// Reveal shows a file selected in File Explorer. explorer.exe needs the path
// quoted inside the /select, argument, so the command line is built by hand;
// it also exits non-zero on success, so its status is ignored.
func Reveal(path string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	_ = cmd.Run()
	return nil
}

// shFileOpStruct is SHFILEOPSTRUCTW (natural alignment on 64-bit Windows).
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete          = 0x3
	fofSilent         = 0x4
	fofNoConfirmation = 0x10
	fofAllowUndo      = 0x40 // to the Recycle Bin instead of deleting
	fofNoErrorUI      = 0x400
)

// Trash moves path to the Recycle Bin, where it can be restored.
func Trash(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	from, err := syscall.UTF16FromString(abs)
	if err != nil {
		return err
	}
	from = append(from, 0) // the list of paths ends with an extra NUL
	op := shFileOpStruct{wFunc: foDelete, pFrom: &from[0], fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		return fmt.Errorf("move to Recycle Bin: error 0x%x", r)
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("move to Recycle Bin: cancelled")
	}
	return nil
}
