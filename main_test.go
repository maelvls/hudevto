package main

import (
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/maelvls/undent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPagePathToFilePath(t *testing.T) {
	t.Run("file exists", func(t *testing.T) {
		root := t.TempDir()
		withContentDir(t, root, "content/article.md")

		p, err := pagePathToFilePath(root, "article")

		require.NoError(t, err)
		require.Equal(t, filepath.Join(root, "content/article.md"), p)
	})

	t.Run("directory with index.md exists", func(t *testing.T) {
		root := t.TempDir()
		withContentDir(t, root, "content/my-article/index.md")

		p, err := pagePathToFilePath(root, "my-article")

		require.NoError(t, err)
		require.Equal(t, filepath.Join(root, "content/my-article/index.md"), p)
	})

	t.Run("neither exists", func(t *testing.T) {
		root := t.TempDir()

		p, err := pagePathToFilePath(root, "notfound")
		require.Error(t, err)
		assert.Empty(t, p)
		assert.Equal(t, undent.Undent(`
			wasn't able to find the source file for the URL path notfound, tried:
			  - as a Markdown file (<root>/content/notfound.md)
			  - as an index file (<root>/content/notfound/index.md)`),
			rmAnsicodes(strings.ReplaceAll(err.Error(), root, "<root>")))

	})

	t.Run("permission error", func(t *testing.T) {
		root := t.TempDir()

		badDir := filepath.Join(root, "content")
		err := os.MkdirAll(badDir, 0000)
		require.NoError(t, err)

		_, err = pagePathToFilePath(root, "bad-article")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "content/bad-article.md: permission denied")
	})
}

// path can be:
// - content/article.md
// - content/other/index.md
func withContentDir(t *testing.T, tmpDir, mdPath string) {
	dir := filepath.Join(tmpDir, path.Dir(mdPath))
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err, "failed to create content directory")

	filePath := filepath.Join(dir, path.Base(mdPath))
	err = os.WriteFile(filePath, []byte("test content"), 0644)
	require.NoError(t, err, "failed to create content file")
}

func rmAnsicodes(s string) string {
	// Remove ANSI escape codes from the string.
	// This is a simple implementation that removes all escape sequences.
	return strings.NewReplacer(
		"\x1b[0;90m", "",
		"\x1b[0m", "",
	).Replace(s)
}

func Test_convertAnchorIDs(t *testing.T) {
	tests := []struct {
		// I initially wanted to use Hugo's SanitizeAnchorName, but it would
		// require a complicated setup. Instead, I pretend that I'm Hugo and I
		// do the conversion myself.
		headingToAnchor map[string]string
		given, expect   string
	}{
		{
			headingToAnchor: map[string]string{"`go get -u` vs. `go.mod` (= *_problem_*)": "go-get--u-vs-gomod--_problem_"},
			given: "# `go get -u` vs. `go.mod` (= *_problem_*)\n" +
				"[`go get -u` vs. `go.mod` (= *_problem_*)](#go-get--u-vs-gomod--_problem_)\n",
			expect: "# `go get -u` vs. `go.mod` (= *_problem_*)\n" +
				"[`go get -u` vs. `go.mod` (= *_problem_*)](#-raw-go-get-u-endraw-vs-raw-gomod-endraw-problem)\n",
		},
		{
			// The closing endraw is always followed by a dash.
			headingToAnchor: map[string]string{"`foo`": "foo"},
			given: "# `foo`\n" +
				"[`foo`](#foo)\n",
			expect: "# `foo`\n" +
				"[`foo`](#-raw-foo-endraw-)\n",
		},
		{
			// Upper case letters are converted to lower case.
			headingToAnchor: map[string]string{"Title Is CAPITAL": "title-is-capital"},
			given: "### Title Is CAPITAL\n" +
				"[Title Is CAPITAL](#title-is-capital)\n",
			expect: "### Title Is CAPITAL\n" +
				"[Title Is CAPITAL](#title-is-capital)\n",
		},
		{
			// It should not affect the anchor links that were manually created.
			headingToAnchor: map[string]string{"Docker proxy vs. Local registry": "docker-proxy-vs-local-registry"},
			given: "## Docker proxy vs. Local registry\n" +
				"[see this section](#docker-proxy-vs-local-registry)\n",
			expect: "## Docker proxy vs. Local registry\n" +
				"[see this section](#docker-proxy-vs-local-registry)\n",
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, tt.expect, convertAnchorIDs("path/to/file.md", tt.given, func(s string) string {
				return tt.headingToAnchor[s]
			}))
		})
	}
}

func Test_addPostURLInHTMLImages(t *testing.T) {
	tests := []struct {
		given, expect string
	}{
		{
			`<img alt="Super example" src="dnat-google-vpc-how-comes-back.svg" width="80%"/>`,
			`<img alt="Super example" src="/you-should-write-comments/dnat-google-vpc-how-comes-back.svg" width="80%"/>`,
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, tt.expect, addPostURLInHTMLImages(tt.given, "/you-should-write-comments/"))
		})
	}
}
