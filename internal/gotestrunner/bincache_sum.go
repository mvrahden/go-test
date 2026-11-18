package gotestrunner

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// sumSuffix names, beside a cached binary, the record of the sum it had
// when gotest last saw the go command link it.
const sumSuffix = ".sum"

// castagnoli is CRC-32C, hardware-accelerated on amd64 and arm64.
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// binarySum is a file's size and CRC-32C. Not its build id: a truncated or
// zero-filled binary keeps that.
type binarySum struct {
	Size int64
	CRC  uint32
}

// sumFile reads path whole and sums it.
func sumFile(path string) (binarySum, error) {
	f, err := os.Open(path)
	if err != nil {
		return binarySum{}, err
	}
	defer f.Close()
	h := crc32.New(castagnoli)
	n, err := io.Copy(h, f)
	if err != nil {
		return binarySum{}, err
	}
	return binarySum{Size: n, CRC: h.Sum32()}, nil
}

// copyFileSum is copyFile, summing what it copies.
func copyFileSum(src, dst string) (binarySum, error) {
	in, err := os.Open(src)
	if err != nil {
		return binarySum{}, err
	}
	defer in.Close()
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	_ = os.Remove(dst)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return binarySum{}, err
	}
	h := crc32.New(castagnoli)
	n, err := io.Copy(io.MultiWriter(out, h), in)
	if err != nil {
		out.Close()
		return binarySum{}, err
	}
	if err := out.Close(); err != nil {
		return binarySum{}, err
	}
	return binarySum{Size: n, CRC: h.Sum32()}, nil
}

// readSum reads binary's record; false when it is absent or malformed.
func readSum(binary string) (binarySum, bool) {
	data, err := os.ReadFile(binary + sumSuffix)
	if err != nil {
		return binarySum{}, false
	}
	var s binarySum
	if n, _ := fmt.Sscanf(string(data), "crc32c=%x size=%d", &s.CRC, &s.Size); n != 2 || string(data) != formatSum(s) {
		return binarySum{}, false
	}
	return s, true
}

// writeSum records s as binary's sum: a temp file in the same dir, then a
// rename, so a reader never sees half a record.
func writeSum(binary string, s binarySum) error {
	tmp, err := os.CreateTemp(filepath.Dir(binary), filepath.Base(binary)+sumSuffix+".tmp-")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(formatSum(s)); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), binary+sumSuffix); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

func formatSum(s binarySum) string {
	return fmt.Sprintf("crc32c=%08x size=%d\n", s.CRC, s.Size)
}
