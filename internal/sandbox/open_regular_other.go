//go:build !linux

// 开发环境用，不能原子性的检查并打开文件，中间存在极短时间窗口可以修改为syslink
package sandbox

import "os"

func openRegularNoFollow(path string) (*os.File, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrWorkspaceFileNotFound
		}
		return nil, nil, err
	}

	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, ErrWorkspaceItemNotRegular
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	return file, info, nil
}

func openRegularForWriteNoFollow(path string, perm os.FileMode) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		}
		return nil, err
	}

	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, ErrWorkspaceItemNotRegular
	}

	return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, perm)
}
