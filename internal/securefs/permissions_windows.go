package securefs

import (
	"os/user"
	"unsafe"

	"golang.org/x/sys/windows"
)

func Private(path string) error {
	current, err := user.Current()
	if err != nil {
		return err
	}
	// Setting a directory's DACL rewrites the inherited entries of everything below it, which
	// takes seconds for a cache of thousands of files; a directory already private is left be.
	if isPrivate(path, current.Uid) {
		return nil
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + current.Uid + ")")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

// isPrivate reports whether path's DACL is the one Private sets: protected, with a single entry
// granting uid full access, inherited by files and directories below. Its text form varies, as
// Windows adds the auto-inherited flag once it has propagated it.
func isPrivate(path, uid string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		return false
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(acl, 0, &ace); err != nil {
		return false
	}
	const inherit = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	const fullAccess = windows.STANDARD_RIGHTS_ALL | 0x1ff
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	return ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE &&
		ace.Header.AceFlags&inherit == inherit &&
		ace.Mask&fullAccess == fullAccess &&
		sid.String() == uid
}
