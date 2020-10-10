// © 2020 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package fileutil provides helper functions
// for working with files and directories.
package fileutil // import "go.astrophena.name/gen/pkg/fileutil"

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyDirContents recursively copies contents
// of the src directory to dst or returns an
// error otherwise.
func CopyDirContents(src, dst string) (err error) {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		sp := strings.TrimPrefix(path, src+string(os.PathSeparator))
		dp := filepath.Join(dst, sp)

		if path == src || info.IsDir() {
			return nil
		}

		dir := filepath.Dir(dp)
		if err := Mkdir(dir); err != nil {
			return err
		}

		if err := CopyFile(path, dp); err != nil {
			return err
		}

		return nil
	})
}

// CopyFile copies the src file to dst or returns
// an error otherwise. Any existing file will be
// overwritten and it will not copy file attributes.
func CopyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

// Exists returns true if a file or directory
// exists and false otherwise.
func Exists(path string) (exists bool) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

// Files returns a slice of files in the directory dir
// recursively, with file extensions exts, or an error.
// If no file extensions are supplied, all files are
// returned.
func Files(dir string, exts ...string) (files []string, err error) {
	return files, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if len(exts) > 0 {
			var extMatches bool
			for _, ext := range exts {
				if filepath.Ext(path) == ext {
					extMatches = true
				}
			}
			if !extMatches {
				return nil
			}
		}

		files = append(files, path)

		return nil
	})
}

// Mkdir creates the directory, if it do not already
// exist or returns an error. It also creates parent
// directories as needed.
func Mkdir(dir string) (err error) {
	if !Exists(dir) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
