//go:build !linux

package pathpolicy

import "os"

type CapturedFile struct{}

func CaptureFileAt(*os.File, string) (*CapturedFile, error) {
	return nil, unsupported()
}

func (*CapturedFile) RemoveIfUnchanged([]byte, int64) (bool, error) {
	return false, unsupported()
}

func (*CapturedFile) Close() error { return unsupported() }
