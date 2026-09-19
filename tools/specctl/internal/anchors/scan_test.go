package anchors

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()

	full := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o600))
}

func TestScan_FindsAnchor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, root, "internal/handlers/on_photo/on_photo_test.go", `package on_photo

import "testing"

// spec: PHOTO-003
func TestOnPhoto_DropsBotEntries(t *testing.T) {}
`)

	got, err := Scan(root)
	require.NoError(t, err)
	require.Len(t, got, 1)

	assert.Equal(t, "PHOTO-003", got[0].ScenarioID)
	assert.Equal(t, "TestOnPhoto_DropsBotEntries", got[0].TestName)
	assert.Contains(t, got[0].File, "on_photo_test.go")
	assert.Equal(t, 6, got[0].Line)
}

func TestScan_MultipleAnchorsOnOneTest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, root, "a_test.go", `package a

import "testing"

// Тест закрывает сразу два сценария.
// spec: CAP-001
// spec: CAP-002
func TestBoth(t *testing.T) {}
`)

	got, err := Scan(root)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "CAP-001", got[0].ScenarioID)
	assert.Equal(t, "CAP-002", got[1].ScenarioID)
	assert.Equal(t, "TestBoth", got[1].TestName)
}

func TestScan_IgnoresNonTestFilesAndNonTestFuncs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, root, "prod.go", `package a

// spec: CAP-001
func Helper() {}
`)
	write(t, root, "b_test.go", `package a

// spec: CAP-002
func helperNotATest() {}
`)

	got, err := Scan(root)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestScan_RejectsMalformedID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, root, "c_test.go", `package a

import "testing"

// spec: photo-3
func TestBroken(t *testing.T) {}
`)

	_, err := Scan(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "photo-3")
	assert.Contains(t, err.Error(), "ожидается формат [PREFIX-NNN]")
}

func TestScan_SkipsVendorAndHidden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, root, "vendor/x/v_test.go", `package x

import "testing"

// spec: CAP-001
func TestVendored(t *testing.T) {}
`)
	write(t, root, ".git/h_test.go", `package h

import "testing"

// spec: CAP-002
func TestHidden(t *testing.T) {}
`)

	got, err := Scan(root)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestScan_SortedByScenarioID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, root, "z_test.go", `package a

import "testing"

// spec: CAP-009
func TestZ(t *testing.T) {}
`)
	write(t, root, "a_test.go", `package a

import "testing"

// spec: CAP-001
func TestA(t *testing.T) {}
`)

	got, err := Scan(root)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "CAP-001", got[0].ScenarioID)
	assert.Equal(t, "CAP-009", got[1].ScenarioID)
}
