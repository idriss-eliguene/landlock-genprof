//go:build gendocs

package main

import "testing"

func TestNormalizeGeneratedCLIPageAnnotatesFences(t *testing.T) {
	input := "### Synopsis\n\n```\nlandlock-genprof observe [flags]\n```\n\n### Examples\n\n```\n  kubectl landlock-genprof observe --pod demo\n```\n\n### Options\n\n```\n  -h, --help   help\n```\n"
	want := "### Synopsis\n\n```text\nlandlock-genprof observe [flags]\n```\n\n### Examples\n\n```sh\n  kubectl landlock-genprof observe --pod demo\n```\n\n### Options\n\n```text\n  -h, --help   help\n```\n"
	if got := normalizeGeneratedCLIPage(input); got != want {
		t.Fatalf("normalized page mismatch:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestNormalizeGeneratedCLIPageLeavesAnnotatedFences(t *testing.T) {
	input := "### Examples\n\n```sh\n  kubectl landlock-genprof version\n```\n"
	if got := normalizeGeneratedCLIPage(input); got != input {
		t.Fatalf("already annotated page changed:\n%s", got)
	}
}
