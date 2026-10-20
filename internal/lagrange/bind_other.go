//go:build !windows || !amd64

package lagrange

import "errors"

func bindNative(string) (string, int, error) {
	return "", 0, errors.New("native backend is not implemented on this platform")
}
