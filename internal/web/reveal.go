package web

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

func reveal(path string, isDir bool) error {
	switch runtime.GOOS {
	case "windows":
		if isDir {
			return exec.Command("explorer.exe", path).Start()
		}
		return exec.Command("explorer.exe", "/select,"+path).Start()
	case "darwin":
		if isDir {
			return exec.Command("open", path).Start()
		}
		return exec.Command("open", "-R", path).Start()
	default:
		dir := path
		if !isDir {
			dir = filepath.Dir(path)
		}
		return exec.Command("xdg-open", dir).Start()
	}
}
