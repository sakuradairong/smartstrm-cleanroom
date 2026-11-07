package rename

import (
	"context"
	"errors"
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

type renameHookStorage struct {
	storage.Storage
	rename func(context.Context, string, string) error
}

func (s renameHookStorage) Rename(ctx context.Context, from, to string) error {
	return s.rename(ctx, from, to)
}

func TestExecuteRestoresOriginalsAfterCancellation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cancelAfter int
		changes     []Change
	}{
		{"staging", 1, []Change{{From: "A.mkv", To: "C.mkv"}, {From: "B.mkv", To: "D.mkv"}}},
		{"finalizing swap", 3, []Change{{From: "A.mkv", To: "B.mkv"}, {From: "B.mkv", To: "A.mkv"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"A.mkv", "B.mkv"} {
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(name), 0o644))
			}
			local, err := storage.NewLocal(root)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			instance := renameHookStorage{Storage: local, rename: func(ctx context.Context, from, to string) error {
				err := local.Rename(ctx, from, to)
				if err == nil {
					calls++
					if calls == tc.cancelAfter {
						cancel()
					}
				}
				return err
			}}

			err = Execute(ctx, instance, "/", tc.changes)

			require.ErrorIs(t, err, context.Canceled)
			for _, name := range []string{"A.mkv", "B.mkv"} {
				data, err := os.ReadFile(filepath.Join(root, name))
				require.NoError(t, err)
				require.Equal(t, name, string(data))
			}
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Len(t, entries, 2, "recovery must leave no temporary or final names")
		})
	}
}

func TestExecuteReportsRecoveryErrors(t *testing.T) {
	for _, tc := range []struct {
		name         string
		failCall     int
		recoveryCall int
	}{
		{"staging restore", 2, 3},
		{"finalized restage", 4, 5},
		{"finalizing restore", 4, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"A.mkv", "B.mkv"} {
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(name), 0o644))
			}
			local, err := storage.NewLocal(root)
			require.NoError(t, err)
			operationErr := errors.New("injected rename failure")
			recoveryErr := errors.New("injected recovery failure")
			calls := 0
			instance := renameHookStorage{Storage: local, rename: func(ctx context.Context, from, to string) error {
				calls++
				if calls == tc.failCall {
					return operationErr
				}
				if calls > tc.failCall {
					_, bounded := ctx.Deadline()
					assert.True(t, bounded, "recovery must have a deadline")
					assert.NoError(t, ctx.Err())
				}
				if calls == tc.recoveryCall {
					return recoveryErr
				}
				return local.Rename(ctx, from, to)
			}}

			err = Execute(context.Background(), instance, "/", []Change{{From: "A.mkv", To: "C.mkv"}, {From: "B.mkv", To: "D.mkv"}})

			require.ErrorIs(t, err, operationErr)
			require.ErrorIs(t, err, recoveryErr)
		})
	}
}
