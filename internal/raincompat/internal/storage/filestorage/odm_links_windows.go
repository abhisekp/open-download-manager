//go:build windows

package filestorage

import (
	"golang.org/x/sys/windows"
	"os"
)

func hasSingleLink(f *os.File) (bool, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return false, err
	}
	return info.NumberOfLinks == 1, nil
}
