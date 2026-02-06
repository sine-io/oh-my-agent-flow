//go:build windows

package console

func parseUnixExitStatus(err error) (exitCode int, signal string, ok bool) {
	return 0, "", false
}
