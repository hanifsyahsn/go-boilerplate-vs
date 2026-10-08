package utility

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectRootFrom(t *testing.T) {
	t.Run("success - go.mod in the starting directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n"), 0o644))

		root := ProjectRootFrom(tmpDir)

		expected, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)
		actual, err := filepath.EvalSymlinks(root)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})

	t.Run("success - go.mod in a parent directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n"), 0o644))

		nestedDir := filepath.Join(tmpDir, "internal", "features", "accessrequest")
		require.NoError(t, os.MkdirAll(nestedDir, 0o755))

		root := ProjectRootFrom(nestedDir)

		expected, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)
		actual, err := filepath.EvalSymlinks(root)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})

	t.Run("success - go.mod several levels up", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n"), 0o644))

		deeplyNestedDir := filepath.Join(tmpDir, "a", "b", "c", "d", "e")
		require.NoError(t, os.MkdirAll(deeplyNestedDir, 0o755))

		root := ProjectRootFrom(deeplyNestedDir)

		expected, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)
		actual, err := filepath.EvalSymlinks(root)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})

	t.Run("panics - go.mod not found before reaching filesystem root", func(t *testing.T) {
		// A tree with no go.mod anywhere in its ancestry within tmpDir's scope.
		// filepath.Dir eventually reaches "/" (or the OS root), where
		// Dir("/") == "/" triggers the panic branch.
		tmpDir := t.TempDir()
		noModDir := filepath.Join(tmpDir, "no", "go", "mod", "here")
		require.NoError(t, os.MkdirAll(noModDir, 0o755))

		// NOTE: this still panics only if there's truly no go.mod above
		// tmpDir up to filesystem root. Since ProjectRootFrom walks all
		// the way up regardless of tmpDir's boundary, and t.TempDir() is
		// typically under an OS temp path without a go.mod ancestor,
		// this holds in CI/sandboxed environments.
		require.Panics(t, func() {
			ProjectRootFrom(noModDir)
		})
	})
}

func TestProjectRoot_projectRootWithGetwd(t *testing.T) {
	t.Run("success - returns module root for a valid working directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n"), 0o644))

		fakeGetwd := func() (string, error) {
			return tmpDir, nil
		}

		root := projectRootWithGetwd(fakeGetwd)

		expected, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)
		actual, err := filepath.EvalSymlinks(root)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})

	t.Run("panics - getwd fails", func(t *testing.T) {
		fakeGetwd := func() (string, error) {
			return "", errors.New("getwd failed")
		}

		require.Panics(t, func() {
			projectRootWithGetwd(fakeGetwd)
		})
	})
}

func TestProjectRoot(t *testing.T) {
	t.Run("success - delegates to os.Getwd and finds this module's root", func(t *testing.T) {
		// This test runs from within the actual repo, so it should find
		// the real go.mod without any directory manipulation.
		root := ProjectRoot()

		_, err := os.Stat(filepath.Join(root, "go.mod"))
		require.NoError(t, err)
	})
}
