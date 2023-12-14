package plugin

import (
	"path/filepath"
)

func Discover(glob, dir string) ([]string, error) {
	var err error

	if !filepath.IsAbs(dir) {
		dir, err = filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
	}

	return filepath.Glob(filepath.Join(dir, glob))
}
