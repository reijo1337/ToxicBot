package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoPackages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	for _, rel := range []string{
		cmdPkg + "/main.go",
		photoPkg + "/on_photo.go",
		photoPkg + "/on_photo_test.go",
		"vendor/x/x.go",
		".hidden/y.go",
		"docs/readme.md",
	} {
		full := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
		require.NoError(t, os.WriteFile(full, []byte("package p"), 0o600))
	}

	got, err := GoPackages(root)
	require.NoError(t, err)
	assert.Equal(t, []string{cmdPkg, photoPkg}, got)
}

func TestGoPackages_TestOnlyDirIsNotAPackage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	full := filepath.Join(root, "internal/only/only_test.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(t, os.WriteFile(full, []byte("package only"), 0o600))

	got, err := GoPackages(root)
	require.NoError(t, err)
	assert.Equal(t, []string{"internal/only"}, got)
}
