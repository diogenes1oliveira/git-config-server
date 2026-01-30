package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// SyncDirs recursively synchronizes two directories.
//
// First, delete all items in the destination that don't match the source: either they don't
// exist in the source, or are files in the destination and directories in the source or vice-versa.
//
// Then copy all files, overwriting. Then, create all directories in the source and recursively
// sync them too
func SyncDirs(src, dst string) error {
	// Delete items in the destination that don't match the source
	err := filepath.Walk(dst, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dst, path)
		if err != nil {
			return fmt.Errorf("failed to relativize path %s inside %s: %w", dst, path, err)
		}
		srcPath := filepath.Join(src, relPath)
		srcInfo, err := os.Stat(srcPath)

		if os.IsNotExist(err) || (srcInfo.IsDir() != info.IsDir()) || (IsExecAny(srcInfo) != IsExecAny(info)) {
			err := os.RemoveAll(path)
			if err != nil {
				return fmt.Errorf("failed to remove dst file or dir %s: %w", dst, err)
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove non-matching dst dir: %w", err)
	}

	// Copy files and create directories from source to destination
	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("failed to relativize %s inside the source %s: %w", src, path, err)
		}
		dstPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			err := os.MkdirAll(dstPath, 0775)
			if err != nil {
				return fmt.Errorf("failed to create dst dir %s: %w", dstPath, err)
			}
		} else if err := copyFile(path, dstPath, info.Mode().Perm()); err != nil {
			return fmt.Errorf("failed to copy source dir %s to %s: %w", path, dstPath, err)
		}
		return nil
	})
	return err
}

// copyFile copies a file from src to dst
func copyFile(src, dst string, mode fs.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}
	if err := dstFile.Close(); err != nil {
		return err
	}

	if mode != 0 {
		return os.Chmod(dst, mode)
	} else {
		return nil
	}
}

func IsExecAny(info os.FileInfo) bool {
	return info.Mode().Perm()&0111 != 0
}
