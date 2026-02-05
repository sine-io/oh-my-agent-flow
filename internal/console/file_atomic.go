package console

import (
	"os"
	"path/filepath"
)

func writeFileAtomicWithPrefix(path string, data []byte, perm os.FileMode, tmpPrefix string) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, tmpPrefix)
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Ensure rename works even if the destination already exists.
	_ = os.Remove(path)
	return os.Rename(tmpPath, path)
}
