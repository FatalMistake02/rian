package desktop

import "os"

func controllingTerminal() (*os.File, *os.File, func(), error) {
	in, err := os.OpenFile("CONIN$", os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		_ = in.Close()
		return nil, nil, nil, err
	}
	return in, out, func() { _ = in.Close(); _ = out.Close() }, nil
}
