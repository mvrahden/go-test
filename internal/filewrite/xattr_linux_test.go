package filewrite_test

import "syscall"

func setXattr(path, name, value string) error {
	return syscall.Setxattr(path, name, []byte(value), 0)
}

func getXattr(path, name string) (string, error) {
	buf := make([]byte, 64)
	n, err := syscall.Getxattr(path, name, buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}
