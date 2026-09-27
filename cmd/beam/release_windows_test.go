package main

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsReleaseStaysConsoleSubsystem(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "windowsgui") {
		t.Fatal("Windows release must be a console application so CMD and PowerShell wait until output is printed")
	}
}
