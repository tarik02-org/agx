package securefs

import (
	"os/user"

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

// isPrivate reports whether path's DACL is the one Private sets. Windows reads it back with the
// auto-inherited flag (AI) once it has propagated it.
func isPrivate(path, uid string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	ace := "(A;OICI;FA;;;" + uid + ")"
	got := sd.String()
	return got == "D:P"+ace || got == "D:PAI"+ace
}
