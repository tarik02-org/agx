package ui

import (
	"os"

	"charm.land/lipgloss/v2"
)

// Classic Windows consoles render escape sequences only once virtual terminal processing is on.
func enableVirtualTerminal(f *os.File) { lipgloss.EnableLegacyWindowsANSI(f) }
