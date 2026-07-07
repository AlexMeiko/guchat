//go:build linux

package sandbox

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openRegularNoFollow(path string) (*os.File, os.FileInfo, error) {
	pathFD, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, nil, ErrWorkspaceFileNotFound
		}
		return nil, nil, err
	}
	defer unix.Close(pathFD)

	var pathStat unix.Stat_t
	if err := unix.Fstat(pathFD, &pathStat); err != nil {
		return nil, nil, err
	}

	if pathStat.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, nil, ErrWorkspaceItemNotRegular
	}

	readFD, err := unix.Open(fmt.Sprintf("/proc/self/fd/%d", pathFD), unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}

	var readStat unix.Stat_t
	if err := unix.Fstat(readFD, &readStat); err != nil {
		unix.Close(readFD)
		return nil, nil, err
	}

	if readStat.Dev != pathStat.Dev || readStat.Ino != pathStat.Ino {
		unix.Close(readFD)
		return nil, nil, ErrWorkspaceItemNotRegular
	}

	file := os.NewFile(uintptr(readFD), path)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}

	return file, info, nil
}

func openRegularForWriteNoFollow(path string, perm os.FileMode) (*os.File, error) {
	pathFD, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, uint32(perm.Perm()))
			if err != nil {
				return nil, err
			}
			return os.NewFile(uintptr(fd), path), nil
		}
		return nil, err
	}
	defer unix.Close(pathFD)

	var pathStat unix.Stat_t
	if err := unix.Fstat(pathFD, &pathStat); err != nil {
		return nil, err
	}
	if pathStat.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, ErrWorkspaceItemNotRegular
	}

	writeFD, err := unix.Open(fmt.Sprintf("/proc/self/fd/%d", pathFD), unix.O_WRONLY|unix.O_TRUNC|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}

	var writeStat unix.Stat_t
	if err := unix.Fstat(writeFD, &writeStat); err != nil {
		unix.Close(writeFD)
		return nil, err
	}
	if writeStat.Dev != pathStat.Dev || writeStat.Ino != pathStat.Ino {
		unix.Close(writeFD)
		return nil, ErrWorkspaceItemNotRegular
	}

	return os.NewFile(uintptr(writeFD), path), nil
}
