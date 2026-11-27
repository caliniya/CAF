package Platform

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// Resolve using the handle to the selected object. This works when a Windows user can
// traverse but not enumerate every ancestor of a permitted workspace or export folder.
func PhysicalPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]uint16, 512)
	for {
		n, e := windows.GetFinalPathNameByHandle(windows.Handle(f.Fd()), &buf[0], uint32(len(buf)), 0)
		if e != nil {
			return "", e
		}
		if n >= uint32(len(buf)) {
			buf = make([]uint16, n+1)
			continue
		}
		v := windows.UTF16ToString(buf[:n])
		if strings.HasPrefix(v, `\\?\UNC\`) {
			return `\\` + v[8:], nil
		}
		return strings.TrimPrefix(v, `\\?\`), nil
	}
}
