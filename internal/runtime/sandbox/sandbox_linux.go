//go:build linux

package sandbox

import (
	"os/exec"
	"sync"
)

var (
	unshareOnce   sync.Once
	unshareResult bool
)

func unshareSupported() bool {
	unshareOnce.Do(func() {
		if _, err := exec.LookPath("unshare"); err != nil {
			unshareResult = false
			return
		}
		cmd := exec.Command("unshare", "--user", "--map-root-user", "true")
		err := cmd.Run()
		unshareResult = err == nil
	})
	return unshareResult
}

func isLinux() bool { return true }
