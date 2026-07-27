#!/bin/bash
# Tests for install_plugins.sh npm implementation
#
# Usage: ./install_plugins_test.sh
#
# Tests:
#   - parse_npmrc(): Registry URL and auth token extraction
#   - verify_integrity(): SHA256/384/512 verification
#   - url_encode(): Scoped package name encoding
#   - download_npm(): Integration test with real registry (optional)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_TMP_DIR=""
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# ============================================================================
# Test Framework
# ============================================================================

setup() {
    TEST_TMP_DIR=$(mktemp -d)
}

teardown() {
    if [[ -n "${TEST_TMP_DIR}" && -d "${TEST_TMP_DIR}" ]]; then
        rm -rf "${TEST_TMP_DIR}"
    fi
}

# Source functions from install_plugins.sh without running main
# We extract function definitions using awk to handle multi-line functions properly
source_functions() {
    local script="${SCRIPT_DIR}/install_plugins.sh"

    # Extract parse_npmrc function
    eval "$(awk '/^parse_npmrc\(\)/{found=1} found{print; if(/^}$/){found=0}}' "$script")"

    # Extract url_encode function
    eval "$(awk '/^url_encode\(\)/{found=1} found{print; if(/^}$/){found=0}}' "$script")"

    # Extract verify_integrity function
    eval "$(awk '/^verify_integrity\(\)/{found=1} found{print; if(/^}$/){found=0}}' "$script")"

    # Extract download_npm function (for integration tests)
    eval "$(awk '/^download_npm\(\)/{found=1} found{print; if(/^}$/){found=0}}' "$script")"
}

assert_equals() {
    local expected="$1"
    local actual="$2"
    local msg="${3:-}"

    if [[ "${expected}" == "${actual}" ]]; then
        return 0
    else
        echo -e "${RED}FAIL${NC}: ${msg}"
        echo "  Expected: '${expected}'"
        echo "  Actual:   '${actual}'"
        return 1
    fi
}

assert_not_empty() {
    local value="$1"
    local msg="${2:-}"

    if [[ -n "${value}" ]]; then
        return 0
    else
        echo -e "${RED}FAIL${NC}: ${msg} - expected non-empty value"
        return 1
    fi
}

assert_file_exists() {
    local file="$1"
    local msg="${2:-}"

    if [[ -f "${file}" ]]; then
        return 0
    else
        echo -e "${RED}FAIL${NC}: ${msg} - file not found: ${file}"
        return 1
    fi
}

run_test() {
    local test_name="$1"
    local test_func="$2"

    TESTS_RUN=$((TESTS_RUN + 1))
    echo -n "Running: ${test_name}... "

    if ${test_func}; then
        echo -e "${GREEN}PASS${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# ============================================================================
# Tests: parse_npmrc()
# ============================================================================

test_parse_npmrc_default_registry() {
    unset NPM_REGISTRY NPM_AUTH_TOKEN NPM_CONFIG_USERCONFIG
    NPM_CONFIG_USERCONFIG="${TEST_TMP_DIR}/nonexistent.npmrc"

    parse_npmrc

    assert_equals "https://registry.npmjs.org" "${NPM_REGISTRY}" "default registry"
}

test_parse_npmrc_custom_registry() {
    unset NPM_REGISTRY NPM_AUTH_TOKEN
    local npmrc="${TEST_TMP_DIR}/.npmrc"

    cat > "${npmrc}" << 'EOF'
registry=https://npm.example.com
EOF

    NPM_CONFIG_USERCONFIG="${npmrc}"
    parse_npmrc

    assert_equals "https://npm.example.com" "${NPM_REGISTRY}" "custom registry"
}

test_parse_npmrc_registry_with_trailing_slash() {
    unset NPM_REGISTRY NPM_AUTH_TOKEN
    local npmrc="${TEST_TMP_DIR}/.npmrc"

    cat > "${npmrc}" << 'EOF'
registry=https://npm.example.com/
EOF

    NPM_CONFIG_USERCONFIG="${npmrc}"
    parse_npmrc

    assert_equals "https://npm.example.com" "${NPM_REGISTRY}" "trailing slash removed"
}

test_parse_npmrc_auth_token() {
    unset NPM_REGISTRY NPM_AUTH_TOKEN
    local npmrc="${TEST_TMP_DIR}/.npmrc"

    cat > "${npmrc}" << 'EOF'
//registry.npmjs.org/:_authToken=npm_MySecretToken123
EOF

    NPM_CONFIG_USERCONFIG="${npmrc}"
    parse_npmrc

    assert_equals "npm_MySecretToken123" "${NPM_AUTH_TOKEN}" "auth token extracted"
}

test_parse_npmrc_registry_and_auth() {
    unset NPM_REGISTRY NPM_AUTH_TOKEN
    local npmrc="${TEST_TMP_DIR}/.npmrc"

    cat > "${npmrc}" << 'EOF'
registry=https://npm.private.com
//npm.private.com/:_authToken=secret_token_456
EOF

    NPM_CONFIG_USERCONFIG="${npmrc}"
    parse_npmrc

    assert_equals "https://npm.private.com" "${NPM_REGISTRY}" "registry with auth" && \
    assert_equals "secret_token_456" "${NPM_AUTH_TOKEN}" "token with registry"
}

test_parse_npmrc_quoted_values() {
    unset NPM_REGISTRY NPM_AUTH_TOKEN
    local npmrc="${TEST_TMP_DIR}/.npmrc"

    cat > "${npmrc}" << 'EOF'
registry="https://npm.quoted.com"
//npm.quoted.com/:_authToken="quoted_token"
EOF

    NPM_CONFIG_USERCONFIG="${npmrc}"
    parse_npmrc

    assert_equals "https://npm.quoted.com" "${NPM_REGISTRY}" "quoted registry" && \
    assert_equals "quoted_token" "${NPM_AUTH_TOKEN}" "quoted token"
}

test_parse_npmrc_file_overrides_env() {
    local npmrc="${TEST_TMP_DIR}/.npmrc"

    cat > "${npmrc}" << 'EOF'
registry=https://npm.fromfile.com
EOF

    # Set env var - but file should override it
    NPM_REGISTRY="https://npm.fromenv.com"
    NPM_CONFIG_USERCONFIG="${npmrc}"

    parse_npmrc

    # File value should override env var (npm behavior: file > env for registry in file)
    assert_equals "https://npm.fromfile.com" "${NPM_REGISTRY}" "file registry overrides env"
}

# ============================================================================
# Tests: url_encode()
# ============================================================================

test_url_encode_scoped_package() {
    local result
    result=$(url_encode "@backstage/plugin-catalog")

    assert_equals "%40backstage%2fplugin-catalog" "${result}" "scoped package encoded"
}

test_url_encode_unscoped_package() {
    local result
    result=$(url_encode "lodash")

    assert_equals "lodash" "${result}" "unscoped package unchanged"
}

test_url_encode_nested_scope() {
    local result
    result=$(url_encode "@org/sub/package")

    assert_equals "%40org%2fsub%2fpackage" "${result}" "nested path encoded"
}

# ============================================================================
# Tests: verify_integrity()
# ============================================================================

test_verify_integrity_sha256_valid() {
    local test_file="${TEST_TMP_DIR}/test.txt"
    echo -n "hello world" > "${test_file}"

    # SHA256 of "hello world" (no newline): echo -n "hello world" | openssl dgst -sha256 -binary | openssl base64 -A
    local integrity="sha256-uU0nuZNNPgilLlLX2n2r+sSE7+N6U4DukIj3rOLvzek="

    if verify_integrity "${test_file}" "${integrity}"; then
        return 0
    else
        echo "SHA256 verification failed"
        return 1
    fi
}

test_verify_integrity_sha256_invalid() {
    local test_file="${TEST_TMP_DIR}/test.txt"
    echo -n "hello world" > "${test_file}"

    # Wrong hash
    local integrity="sha256-wronghashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

    if verify_integrity "${test_file}" "${integrity}" 2>/dev/null; then
        echo "Should have failed but passed"
        return 1
    else
        return 0  # Expected to fail
    fi
}

test_verify_integrity_sha512_valid() {
    local test_file="${TEST_TMP_DIR}/test.txt"
    echo -n "hello world" > "${test_file}"

    # SHA512 of "hello world" (no newline)
    local integrity="sha512-MJ7MSJwS1utMxA9QyQLytNDtd+5RGnx6m808qG1M2G+YndNbxf9JlnDaNCVbRbDP2DDoH2Bdz33FVC6TrpzXbw=="

    if verify_integrity "${test_file}" "${integrity}"; then
        return 0
    else
        echo "SHA512 verification failed"
        return 1
    fi
}

test_verify_integrity_empty_skips() {
    local test_file="${TEST_TMP_DIR}/test.txt"
    echo -n "anything" > "${test_file}"

    # Empty integrity should pass (skip verification)
    if verify_integrity "${test_file}" ""; then
        return 0
    else
        echo "Empty integrity should skip verification"
        return 1
    fi
}

test_verify_integrity_skip_env() {
    local test_file="${TEST_TMP_DIR}/test.txt"
    echo -n "hello world" > "${test_file}"

    # Wrong hash but SKIP_INTEGRITY_CHECK=true
    local integrity="sha256-wronghashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

    SKIP_INTEGRITY_CHECK=true
    local result
    if verify_integrity "${test_file}" "${integrity}"; then
        result=0
    else
        result=1
    fi
    unset SKIP_INTEGRITY_CHECK

    if [[ ${result} -eq 0 ]]; then
        return 0
    else
        echo "SKIP_INTEGRITY_CHECK should bypass verification"
        return 1
    fi
}

test_verify_integrity_unsupported_algorithm() {
    local test_file="${TEST_TMP_DIR}/test.txt"
    echo -n "hello" > "${test_file}"

    # MD5 is not supported
    local integrity="md5-XUFAKrxLKna5cZ2REBfFkg=="

    if verify_integrity "${test_file}" "${integrity}" 2>/dev/null; then
        echo "MD5 should not be supported"
        return 1
    else
        return 0  # Expected to fail
    fi
}

# ============================================================================
# Integration Tests (optional, requires network)
# ============================================================================

test_download_npm_real_package() {
    if [[ "${RUN_INTEGRATION_TESTS:-false}" != "true" ]]; then
        echo -e "${YELLOW}SKIP${NC} (set RUN_INTEGRATION_TESTS=true to enable)"
        return 0
    fi

    local plugin_dir="${TEST_TMP_DIR}/is-odd"

    # Run the actual script in bash (not eval'd functions) to ensure bash semantics
    # Create a test input file and run install_plugins.sh
    local test_input="${TEST_TMP_DIR}/packages.txt"
    echo "is-odd@3.0.1" > "${test_input}"

    # Run install_plugins.sh with our test input
    if INPUT_FILE="${test_input}" OUTPUT_DIR="${TEST_TMP_DIR}" PARALLEL_JOBS=1 \
       bash "${SCRIPT_DIR}/install_plugins.sh" >/dev/null 2>&1; then
        assert_file_exists "${plugin_dir}/package.json" "package.json exists"
    else
        echo "install_plugins.sh failed"
        return 1
    fi
}

# ============================================================================
# Main
# ============================================================================

main() {
    echo "============================================"
    echo "install_plugins.sh NPM Implementation Tests"
    echo "============================================"
    echo ""

    # Setup
    setup
    source_functions

    # parse_npmrc tests
    echo "--- parse_npmrc() tests ---"
    run_test "default registry" test_parse_npmrc_default_registry
    run_test "custom registry" test_parse_npmrc_custom_registry
    run_test "registry trailing slash" test_parse_npmrc_registry_with_trailing_slash
    run_test "auth token" test_parse_npmrc_auth_token
    run_test "registry and auth" test_parse_npmrc_registry_and_auth
    run_test "quoted values" test_parse_npmrc_quoted_values
    run_test "file overrides env" test_parse_npmrc_file_overrides_env
    echo ""

    # url_encode tests
    echo "--- url_encode() tests ---"
    run_test "scoped package" test_url_encode_scoped_package
    run_test "unscoped package" test_url_encode_unscoped_package
    run_test "nested scope" test_url_encode_nested_scope
    echo ""

    # verify_integrity tests
    echo "--- verify_integrity() tests ---"
    run_test "sha256 valid" test_verify_integrity_sha256_valid
    run_test "sha256 invalid" test_verify_integrity_sha256_invalid
    run_test "sha512 valid" test_verify_integrity_sha512_valid
    run_test "empty skips" test_verify_integrity_empty_skips
    run_test "skip env var" test_verify_integrity_skip_env
    run_test "unsupported algorithm" test_verify_integrity_unsupported_algorithm
    echo ""

    # Integration tests
    echo "--- Integration tests ---"
    run_test "download real npm package" test_download_npm_real_package
    echo ""

    # Teardown
    teardown

    # Summary
    echo "============================================"
    echo "Results: ${TESTS_PASSED}/${TESTS_RUN} passed"
    if [[ ${TESTS_FAILED} -gt 0 ]]; then
        echo -e "${RED}${TESTS_FAILED} tests failed${NC}"
        exit 1
    else
        echo -e "${GREEN}All tests passed${NC}"
        exit 0
    fi
}

main "$@"
