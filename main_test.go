package main

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
