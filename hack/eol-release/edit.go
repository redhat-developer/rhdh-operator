package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func removeNightlyBranch(content, branch string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*- ` + regexp.QuoteMeta(branch) + `[ \t]*\n`)
	return re.ReplaceAllString(content, "")
}

func removeUpgradePairs(content, branch string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*- from_branch: (\S+)\n[ \t]*to_branch: (\S+)\n`)
	return re.ReplaceAllStringFunc(content, func(block string) string {
		m := re.FindStringSubmatch(block)
		if len(m) == 3 && (m[1] == branch || m[2] == branch) {
			return ""
		}
		return block
	})
}

func renovateAlreadyDisabled(content, branch string) bool {
	needle := fmt.Sprintf(`"matchBaseBranches": [%q]`, branch)
	return strings.Contains(content, needle)
}

func prependRenovateDisable(content, branch string) (string, error) {
	if renovateAlreadyDisabled(content, branch) {
		return content, nil
	}
	const marker = `"packageRules": [`
	idx := strings.Index(content, marker)
	if idx < 0 {
		return "", fmt.Errorf("packageRules not found in renovate.json")
	}
	insertAt := idx + len(marker)
	rule := fmt.Sprintf("\n    {\n      \"matchBaseBranches\": [%q],\n      \"enabled\": false\n    },", branch)
	out := content[:insertAt] + rule + content[insertAt:]
	if !json.Valid([]byte(out)) {
		return "", fmt.Errorf("renovate.json would be invalid after insert")
	}
	return out, nil
}

func changed(before, after string) bool {
	return before != after
}
