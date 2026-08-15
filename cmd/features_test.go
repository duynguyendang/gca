package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestResolveDiff_Inline(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("diff", "", "")
	cmd.Flags().String("diff-file", "", "")
	cmd.Flags().String("base", "", "")
	cmd.Flags().String("head", "", "")
	require.NoError(t, cmd.Flags().Set("diff", "inline-diff-content"))
	got, err := resolveDiff(cmd)
	require.NoError(t, err)
	require.Equal(t, "inline-diff-content", got)
}

func TestResolveDiff_DiffFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pr.diff")
	require.NoError(t, os.WriteFile(path, []byte("diff-content-from-file"), 0o644))

	cmd := &cobra.Command{}
	cmd.Flags().String("diff", "", "")
	cmd.Flags().String("diff-file", "", "")
	cmd.Flags().String("base", "", "")
	cmd.Flags().String("head", "", "")
	require.NoError(t, cmd.Flags().Set("diff-file", path))
	got, err := resolveDiff(cmd)
	require.NoError(t, err)
	require.Equal(t, "diff-content-from-file", got)
}

func TestResolveDiff_MissingSource(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("diff", "", "")
	cmd.Flags().String("diff-file", "", "")
	cmd.Flags().String("base", "", "")
	cmd.Flags().String("head", "", "")
	_, err := resolveDiff(cmd)
	require.Error(t, err)
}

func TestResolveDiff_Git(t *testing.T) {
	// Point the command at the real repo so git diff resolves.
	cmd := &cobra.Command{}
	cmd.Flags().String("diff", "", "")
	cmd.Flags().String("diff-file", "", "")
	cmd.Flags().String("base", "", "")
	cmd.Flags().String("head", "", "")
	require.NoError(t, cmd.Flags().Set("base", "HEAD"))
	got, err := resolveDiff(cmd)
	require.NoError(t, err)
	require.NotEmpty(t, got)
}

func TestFetchWithTimeout_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("advisory-data"))
	}))
	defer srv.Close()

	got, err := fetchWithTimeout(srv.URL, 0)
	require.NoError(t, err)
	require.Equal(t, "advisory-data", string(got))
}

func TestFetchWithTimeout_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := fetchWithTimeout(srv.URL, 0)
	require.Error(t, err)
}

func TestFeatureCommandsRegistered(t *testing.T) {
	for _, name := range []string{"report", "impact", "advisories"} {
		c, _, err := rootCmd.Find([]string{name})
		require.NoError(t, err, "command %q not found", name)
		require.NotNil(t, c)
	}
	// advisories has update + show subcommands.
	for _, sub := range []string{"update", "show"} {
		c, _, err := rootCmd.Find([]string{"advisories", sub})
		require.NoError(t, err, "subcommand advisories %s not found", sub)
		require.NotNil(t, c)
	}
}
