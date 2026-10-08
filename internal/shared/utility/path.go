package utility

import (
	"os"
	"path/filepath"
)

type getwdFunc func() (string, error)

// ProjectRootFrom walks up from startDir until it finds a directory
// containing go.mod, and returns that directory. Panics if no go.mod
// is found before reaching the filesystem root.
func ProjectRootFrom(startDir string) string {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
}

// projectRootWithGetwd is the testable core of ProjectRoot, with the
// working-directory lookup injected rather than hardcoded.
func projectRootWithGetwd(getwd getwdFunc) string {
	dir, err := getwd()
	if err != nil {
		panic(err)
	}
	return ProjectRootFrom(dir)
}

// ProjectRoot returns the root directory of the current Go module,
// found by walking up from the current working directory until a
// go.mod file is found.
func ProjectRoot() string {
	return projectRootWithGetwd(os.Getwd)
}
