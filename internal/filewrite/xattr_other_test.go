//go:build !linux

package filewrite_test

import "errors"

var errNoXattr = errors.New("extended attributes are tested on Linux")

func setXattr(string, string, string) error { return errNoXattr }

func getXattr(string, string) (string, error) { return "", errNoXattr }
