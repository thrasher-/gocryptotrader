#!/usr/bin/env bash
# Miscellaneous repository quality checks replicated from .github/workflows/misc.yml

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

EXIT_CODE=0

is_ci() { [[ "${CI:-}" == "true" || "${GITHUB_ACTIONS:-}" == "true" ]]; }

log() { printf '%s\n' "$*"; }
section() { log "[+] $*:"; }
annotate_error() { if is_ci; then printf '::error::%s\n' "$*"; else printf 'ERROR: %s\n' "$*"; fi; }

fail() { annotate_error "$1"; EXIT_CODE=1; }

have_grep_P() { echo 'test' | grep -P 'test' >/dev/null 2>&1; }

check_currency_newpair() {
  section "Check currency.NewPair(currency.BTC, currency.USD/USDT) occurrences"
  if grep -r -n -E "currency.NewPair\(currency.BTC, currency.USDT?\)" . >/dev/null 2>&1; then
    grep -r -n --color=always -E "currency.NewPair\(currency.BTC, currency.USDT?\)" . || true
    fail "Replace currency.NewPair(BTC, USD*) with currency.NewBTCUSD*()"
  else
    log "Pass"
  fi
}

check_missing_assert_f_variant() {
  section "Check missing testify *f format variants"
  local pattern='(assert|require)\.[A-Za-z_]\w*?(?<!f)\((?:(?!fmt\.Sprintf).)*%.*'
  if have_grep_P; then
    if grep -r -n -P --include='*.go' "$pattern" . >/dev/null 2>&1; then
      grep -r -n -P --include='*.go' --color=always "$pattern" . || true
      fail "Replace func with the corresponding ...f variant (e.g. Equalf, Errorf)"
    else
      log "Pass"
    fi
  else
    log "Skipping (grep -P unsupported); run in CI or install PCRE grep for full check"
  fi
}

check_quoted_percent_s() {
  section "Check quoted/backticked %s format specifiers"
  if grep -r -n --include='*.go' -E "[\`']%s[\`']" . >/dev/null 2>&1; then
    grep -r -n --include='*.go' --color=always -E "[\`']%s[\`']" . || true
    fail "Replace '%s' or \`%s\` with %q in format strings"
  else
    log "Pass"
  fi
}

check_testify_message_consistency() {
  section "Check testify message wording (require: must / assert: should)"
  local bad=0
  log "Scanning for 'should' in require messages..."
  if grep -r -n --include='*.go' -E "require\.[A-Za-z0-9_]+.*\"[^\"]*should[^\"]*\"" . >/dev/null 2>&1; then
    grep -r -n --include='*.go' --color=always -E "require\.[A-Za-z0-9_]+.*\"[^\"]*should[^\"]*\"" . || true
    bad=1
  fi
  log "Scanning for 'must' in assert messages..."
  if grep -r -n --include='*.go' -E "assert\.[A-Za-z0-9_]+.*\"[^\"]*must[^\"]*\"" . >/dev/null 2>&1; then
    grep -r -n --include='*.go' --color=always -E "assert\.[A-Za-z0-9_]+.*\"[^\"]*must[^\"]*\"" . || true
    bad=1
  fi
  if (( bad )); then
    fail "Replace 'should' in require messages and 'must' in assert messages"
  else
    log "Pass"
  fi
}

check_errors_is_nil() {
  section "Check errors.Is(err, nil) usage"
  if grep -r -n --include='*_test.go' -E "errors.Is\([^,]+, nil" . >/dev/null 2>&1; then
    grep -r -n --include='*_test.go' --color=always -E "errors.Is\([^,]+, nil" . || true
    fail "Replace errors.Is(err, nil) with testify equivalents"
  else
    log "Pass"
  fi
}

check_not_errors_is() {
  section "Check !errors.Is(err, target) usage"
  local pattern='!errors\.Is\(\s*[^,]+\s*,\s*[^)]+\s*\)'
  if have_grep_P; then
    if grep -r -n --include='*_test.go' -P "$pattern" . >/dev/null 2>&1; then
      grep -r -n --include='*_test.go' -P --color=always "$pattern" . || true
      fail "Replace !errors.Is(err, target) with testify equivalents"
    else
      log "Pass"
    fi
  else
    # Basic fallback (may overmatch)
    if grep -r -n --include='*_test.go' -E "!errors.Is\(" . >/dev/null 2>&1; then
      grep -r -n --include='*_test.go' --color=always -E "!errors.Is\(" . || true
      fail "Replace !errors.Is(err, target) with testify equivalents"
    else
      log "Pass"
    fi
  fi
}

check_invisible_unicode() {
  section "Check for invisible Unicode characters"
  local whitelist="${WHITELIST:-}" # maintain compatibility with original workflow env var
  local pattern
  if [[ -z "$whitelist" ]]; then
    pattern='(?!\x20)[\p{Cf}\p{Z}\p{M}]'
  else
    pattern="(?![\x20$whitelist])[\p{Cf}\p{Z}\p{M}]"
  fi
  if grep -r -n -I --exclude-dir=.git -P "$pattern" . >/dev/null 2>&1; then
    grep -r -n -I --exclude-dir=.git -P --color=always "$pattern" . || true
    fail "Remove zero-width/format, separator or combining-mark characters"
  else
    log "Pass"
  fi
}

check_configs_json() {
  section "Check config JSON formatting (sorted exchanges)"
  local files=("config_example.json" "testdata/configtest.json")
  if ! command -v jq >/dev/null 2>&1; then
    fail "jq not installed (needed for config formatting). Install jq or run 'make lint_configs' after installing."
    return
  fi
  local f processed
  local had_diff=0
  for f in "${files[@]}"; do
    if [[ ! -f "$f" ]]; then
      log "Skipping missing $f"
      continue
    fi
    processed="${f%.*}_processed.${f##*.}"
    jq '.exchanges |= sort_by(.name)' --indent 1 "$f" > "$processed"
    if ! diff "$f" "$processed" >/dev/null; then
      log "Diff for $f:" && diff -u "$f" "$processed" || true
      had_diff=1
    else
      log "No differences in $f 🌞"
    fi
    rm -f "$processed" 2>/dev/null || true
  done
  if (( had_diff )); then
    fail "Run 'make lint_configs' to apply sorting"
  else
    log "Pass"
  fi
}

check_modernise() {
  section "Check Go modernise tool issues"
  if ! command -v modernize >/dev/null 2>&1; then
    log "Installing modernize tool..."
    go install golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest >/dev/null 2>&1 || {
      fail "Failed to install modernize tool"; return; }
  fi
  if ! modernize -test ./...; then
    fail "Modernize tool reported issues"
  else
    log "Pass"
  fi
}

usage() {
  cat <<EOF
Usage: $0 [options]

Options:
  --skip-modernize   Skip modernize tool check
  --only NAME        Run only the specified check (can repeat)
  --list             List check names
  -h, --help         Show this help

Checks:
  currency_newpair
  missing_assert_f
  quoted_percent_s
  testify_messages
  errors_is_nil
  not_errors_is
  invisible_unicode
  configs_json
  modernize
EOF
}

ALL_CHECKS=(
  currency_newpair
  missing_assert_f
  quoted_percent_s
  testify_messages
  errors_is_nil
  not_errors_is
  invisible_unicode
  configs_json
  modernize
)

RUN_CHECKS=()
SKIP_MODERNIZE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-modernize) SKIP_MODERNIZE=1; shift ;;
    --only) shift; [[ $# -gt 0 ]] || { log "--only requires a value"; exit 2; }; RUN_CHECKS+=("$1"); shift ;;
    --list) printf '%s\n' "${ALL_CHECKS[@]}"; exit 0 ;;
    -h|--help) usage; exit 0 ;;
    *) log "Unknown argument: $1"; usage; exit 2 ;;
  esac
done

if (( ${#RUN_CHECKS[@]} == 0 )); then
  RUN_CHECKS=(${ALL_CHECKS[@]})
fi

for chk in "${RUN_CHECKS[@]}"; do
  case "$chk" in
    currency_newpair)        check_currency_newpair ;;
    missing_assert_f)        check_missing_assert_f_variant ;;
    quoted_percent_s)        check_quoted_percent_s ;;
    testify_messages)        check_testify_message_consistency ;;
    errors_is_nil)           check_errors_is_nil ;;
    not_errors_is)           check_not_errors_is ;;
    invisible_unicode)      check_invisible_unicode ;;
    configs_json)            check_configs_json ;;
    modernize)               if (( ! SKIP_MODERNIZE )); then check_modernise; else log "Skipping modernize"; fi ;;
    *) log "Unknown check name: $chk"; EXIT_CODE=2 ;;
  esac
done

exit ${EXIT_CODE}
