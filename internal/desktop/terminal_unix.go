//go:build !windows

package desktop

import "os"

func controllingTerminal() (*os.File, *os.File, func(), error) {
	file, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	return file, file, func() { _ = file.Close() }, nil
}
