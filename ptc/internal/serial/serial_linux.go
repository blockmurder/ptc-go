// +build linux

package serial

// #include <linux/serial.h>
import "C"

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type Port struct {
	f *os.File
}

func (p *Port) Read(b []byte) (n int, err error)  { return p.f.Read(b) }
func (p *Port) Write(b []byte) (n int, err error) { return p.f.Write(b) }
func (p *Port) Close() error                      { return p.f.Close() }

func Open(path string, baudrate int) (*Port, error) {
	f, err := os.OpenFile(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0666)
	if err != nil {
		return nil, err
	}

	readTimeout := time.Minute
	rate := unix.B38400
	fd := f.Fd()
	vmin, vtime := posixTimeoutValues(readTimeout)
	t := unix.Termios{
		Iflag:  unix.IGNPAR,
		Cflag:  uint32(unix.CS8 | unix.CLOCAL | unix.CREAD | rate),
		Ispeed: uint32(rate),
		Ospeed: uint32(rate),
	}
	t.Cc[unix.VMIN] = vmin
	t.Cc[unix.VTIME] = vtime

	if _, _, errno := unix.Syscall6(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.TCSETS),
		uintptr(unsafe.Pointer(&t)),
		0,
		0,
		0,
	); errno != 0 {
		return nil, errno
	}

	if err = setCustomDivisor(int(fd), 29); err != nil {
		return nil, fmt.Errorf("Could not set custom divisor: %v", err)
	}

	if err = unix.SetNonblock(int(fd), false); err != nil {
		return nil, err
	}

	return &Port{f: f}, nil
}

func setCustomDivisor(fd int, div int) error {
	var sstruct C.struct_serial_struct
	if _, _, errno := unix.Syscall6(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.TIOCGSERIAL),
		uintptr(unsafe.Pointer(&sstruct)),
		0,
		0,
		0,
	); errno != 0 {
		return errno
	}

	sstruct.custom_divisor = 29
	sstruct.flags |= C.ASYNC_SPD_CUST

	if _, _, errno := unix.Syscall6(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.TIOCSSERIAL),
		uintptr(unsafe.Pointer(&sstruct)),
		0,
		0,
		0,
	); errno != 0 {
		return errno
	}
	return nil
}

// Converts the timeout values for Linux / POSIX systems
//
// copied from github.com/tarm/serial
func posixTimeoutValues(readTimeout time.Duration) (vmin uint8, vtime uint8) {
	const MAXUINT8 = 1<<8 - 1 // 255
	// set blocking / non-blocking read
	var minBytesToRead uint8 = 1
	var readTimeoutInDeci int64
	if readTimeout > 0 {
		// EOF on zero read
		minBytesToRead = 0
		// convert timeout to deciseconds as expected by VTIME
		readTimeoutInDeci = (readTimeout.Nanoseconds() / 1e6 / 100)
		// capping the timeout
		if readTimeoutInDeci < 1 {
			// min possible timeout 1 Deciseconds (0.1s)
			readTimeoutInDeci = 1
		} else if readTimeoutInDeci > MAXUINT8 {
			// max possible timeout is 255 deciseconds (25.5s)
			readTimeoutInDeci = MAXUINT8
		}
	}
	return minBytesToRead, uint8(readTimeoutInDeci)
}
