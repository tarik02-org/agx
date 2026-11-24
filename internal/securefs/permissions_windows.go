package securefs

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func MakePrivate(path string) error {
	sid, err := currentSID()
	if err != nil {
		return err
	}
	// Setting a directory's DACL rewrites the inherited entries of everything below it, which
	// takes seconds for a large tree; a directory already private is left be.
	if isPrivate(path, sid) {
		return nil
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")")
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

// currentSID reads the user from the process token; looking the user up by name can ask a domain
// controller.
func currentSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// isPrivate reports whether path's DACL is the one MakePrivate sets: protected, with a single
// entry granting owner full access, inherited by files and directories below. Its text form
// varies, as Windows adds the auto-inherited flag once it has propagated it.
func isPrivate(path, owner string) bool {
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
		sid.String() == owner
}
