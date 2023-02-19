package main

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_convertAnchorIDs(t *testing.T) {
	tests := []struct {
		given, expect string
	}{
		{
			"[`go get -u` vs. `go.mod` (= *_problem_*)](#go-get--u-vs-gomod--_problem_)",
			"[`go get -u` vs. `go.mod` (= *_problem_*)](#-raw-go-get-u-endraw-vs-raw-gomod-endraw-problem)",
		},
		{
			// The closing endraw is always followed by a dash.
			"[`foo`](#foo)",
			"[`foo`](#-raw-foo-endraw-)",
		},
		{
			// Upper case letters are converted to lower case.
			"[Title Is CAPITAL](#title-is-capital)",
			"[Title Is CAPITAL](#title-is-capital)",
		},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, tt.expect, convertAnchorIDs(tt.given))
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
