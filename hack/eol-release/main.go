package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	tag := flag.String("tag", "", "GA tag that just shipped, e.g. 2.1.0")
	dryRun := flag.Bool("dry-run", false, "print planned edits, do not write")
	write := flag.Bool("write", false, "write edits to disk")
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	if *tag == "" {
		fmt.Fprintln(os.Stderr, "--tag is required")
		os.Exit(2)
	}
	if !*dryRun && !*write {
		*dryRun = true
	}

	tags, err := gitGATags(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	eol, ok, err := eolRelease(*tag, tags)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if !ok {
		fmt.Printf("not enough GA history for %s; nothing to do\n", *tag)
		os.Exit(0)
	}
	fmt.Printf("tag=%s eol=%s\n", *tag, eol)
	writeGitHubEnv("TAG", *tag)
	writeGitHubEnv("EOL_BRANCH", eol)
	writeGitHubEnv("EOL_VERSION", strings.TrimPrefix(eol, "release-"))

	files := []struct {
		rel  string
		edit func(string) (string, error)
	}{
		{".github/workflows/nightly.yaml", func(s string) (string, error) {
			return removeNightlyBranch(s, eol), nil
		}},
		{".github/workflows/nightly-upgrade-test.yaml", func(s string) (string, error) {
			return removeUpgradePairs(s, eol), nil
		}},
		{".github/renovate.json", func(s string) (string, error) {
			return prependRenovateDisable(s, eol)
		}},
	}

	any := false
	for _, f := range files {
		path := filepath.Join(*root, f.rel)
		in, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		out, err := f.edit(string(in))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !changed(string(in), out) {
			fmt.Printf("unchanged %s\n", f.rel)
			continue
		}
		any = true
		fmt.Printf("update %s\n", f.rel)
		if *write && !*dryRun {
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}

	if !any {
		fmt.Println("nothing to do")
		os.Exit(0)
	}
	if *dryRun {
		fmt.Println("dry-run: no files written")
	}
}

func gitGATags(root string) ([]string, error) {
	cmd := exec.Command("git", "tag", "--list")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\n"), nil
}

func writeGitHubEnv(k, v string) {
	p := os.Getenv("GITHUB_ENV")
	if p == "" {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s=%s\n", k, v)
}
