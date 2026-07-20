package rename

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanModes(t *testing.T) {
	entries := []storage.Entry{{Name: "Show.S1E2.mkv"}, {Name: "Show.S1E10.mkv"}}
	sequence, err := Plan(entries, Options{Mode: "sequence", Prefix: "S01E", Start: 1, Width: 2, PreserveExtension: true})
	require.NoError(t, err)
	assert.Equal(t, "S01E01.mkv", sequence[0].To)
	assert.Equal(t, "S01E02.mkv", sequence[1].To)

	regex, err := Plan(entries, Options{Mode: "regex", Pattern: `Show\.`, Replacement: ""})
	require.NoError(t, err)
	assert.Len(t, regex, 2)

	magic, err := Plan(entries, Options{Mode: "magic", Template: "{title} - S{season}E{episode}{ext}"})
	require.NoError(t, err)
	require.Len(t, magic, 2)
	assert.Equal(t, "Show - S01E02.mkv", magic[0].To)
}

func TestPlanRejectsCollision(t *testing.T) {
	_, err := Plan([]storage.Entry{{Name: "one.mkv"}, {Name: "two.mkv"}}, Options{Mode: "regex", Pattern: `.*`, Replacement: "same.mkv"})
	require.Error(t, err)
}

func TestExecuteSupportsNameSwap(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "A.mkv"), []byte("A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "B.mkv"), []byte("B"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	require.NoError(t, Execute(context.Background(), local, "/", []Change{{From: "A.mkv", To: "B.mkv"}, {From: "B.mkv", To: "A.mkv"}}))
	a, err := os.ReadFile(filepath.Join(root, "A.mkv"))
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(root, "B.mkv"))
	require.NoError(t, err)
	assert.Equal(t, "B", string(a))
	assert.Equal(t, "A", string(b))
}
