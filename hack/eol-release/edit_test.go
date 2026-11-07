package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRemoveNightlyBranch(t *testing.T) {
	got := removeNightlyBranch(read(t, "nightly.yaml"), "release-1.9")
	if strings.Contains(got, "- release-1.9") {
		t.Fatal("release-1.9 still in nightly matrix")
	}
	for _, keep := range []string{"- main", "- release-2.1", "- release-1.10"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("missing %s", keep)
		}
	}
}

func TestRemoveUpgradePairs(t *testing.T) {
	got := removeUpgradePairs(read(t, "nightly-upgrade-test.yaml"), "release-1.9")
	if strings.Contains(got, "release-1.9") {
		t.Fatal("release-1.9 still in upgrade matrix")
	}
	if !strings.Contains(got, "from_branch: release-1.10") || !strings.Contains(got, "to_branch: release-2.1") {
		t.Fatal("kept 1.10 -> 2.1")
	}
	if !strings.Contains(got, "from_branch: release-2.1") || !strings.Contains(got, "to_branch: main") {
		t.Fatal("kept 2.1 -> main")
	}
}

func TestPrependRenovateDisable(t *testing.T) {
	got, err := prependRenovateDisable(read(t, "renovate.json"), "release-1.9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"matchBaseBranches": ["release-1.9"]`) {
		t.Fatal("disable rule missing")
	}
	again, err := prependRenovateDisable(got, "release-1.9")
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatal("second run must be idempotent")
	}
}
