package main

import (
	"slices"
	"testing"
)

func Test_permittedPackages(t *testing.T) {
	perm := permittedPackages()
	if slices.Contains(perm, "") {
		t.Error("empty string not allowed")
	}
}
