#!/usr/bin/env bash
# AbleOps SDK — 단계별 검증 러너 · **검증 규칙의 단일 출처**
#
#   bash scripts/verify.sh fast      # gofmt · build · vet · test -short                       — 개발 루프(수 초)
#   bash scripts/verify.sh full      # + 전체 테스트(외부 consumer 포함) · 경계 게이트 이름 확인
#                                    #   · 예제 빌드 · 의존성 최소성 · CLAUDE.md 예산          — 작업 종료·PR 전 1회. CI 가 이것을 돈다
#   bash scripts/verify.sh release   # + SDKVersion/CHANGELOG/태그 일치 · go mod tidy 무변경 · 작업트리 clean — 태그 직전 1회
#
# ⚠ 이 스크립트는 판정 규칙을 새로 만들지 않는다. 실행하는 것은 전부 기존 게이트
#   (gofmt · go build/vet/test · extension/v1 의 경계 테스트)이며, 범위만 좁힐 뿐 어떤 단계도 약한 버전으로 대체하지 않는다.
# ⚠ 게이트 목록(GATES)·허용 모듈 정규식은 **여기에만** 둔다. ci.yml 에 복제하면 한쪽이 조용히 어긋난다.
# ⚠ Core 저장소의 sdk/ 스테이징 안에서 돌릴 때는 `GOWORK=off bash scripts/verify.sh <수준>`. 독립 저장소에는 go.work 가 없다.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

LEVEL="${1:-}"
case "$LEVEL" in
  fast|full|release) ;;
  *) echo "사용법: bash scripts/verify.sh <fast|full|release>"; exit 2 ;;
esac

FAILED=0
RESULTS=()

# step <이름> <명령...> — 실패해도 계속 진행하고 마지막에 한 번에 보고한다.
# (첫 실패에서 멈추면 "고치고 다시 돌리기"를 실패 개수만큼 반복하게 된다.)
step() {
  local name="$1"; shift
  echo ""
  echo "── $name ──────────────────────────────────────────"
  if "$@"; then
    RESULTS+=("통과     $name")
  else
    RESULTS+=("실패     $name")
    FAILED=1
  fi
}

# fail <메시지> — GitHub Actions 주석 형식. 로컬에서는 그냥 한 줄이다.
fail() { echo "::error::$*"; return 1; }

# ── 단계 정의 ──────────────────────────────────────────────────────────────

# 포맷은 컴파일 없이 1초 안에 끝난다 — 가장 싼 검사부터.
gofmt_check() {
  local files
  files=$(gofmt -l .)
  [ -z "$files" ] && { echo "gofmt 통과"; return 0; }
  echo "$files"
  fail "gofmt 미적용 파일이 있습니다."
}

# 경계 게이트 — 이 SDK 의 존재 이유에 해당하는 테스트는 **이름으로** 실행·통과를 확인한다.
# `go test ./...` 초록불만으로는 테스트가 개명·삭제되어 조용히 사라진 것을 알 수 없다.
GATES=(
  TestSDKModulePathIsStandalone TestSDKModuleDoesNotRequireCore TestSDKModuleRequiresStayMinimal
  TestSDKDoesNotImportCore TestSDKDoesNotImportInternal TestSDKHasNoUnexpectedDependency TestSDKDoesNotImportChi
  TestPublicAPISnapshot TestSDKVersionMatchesChangelog TestSDKVersionIsSemver TestAPIVersionMatchesModuleMajor
  TestExternalConsumerCompiles
)
gates_check() {
  local pattern log tn
  pattern="^($(IFS='|'; echo "${GATES[*]}"))$"
  log="$(mktemp)"
  go test ./extension/v1 -count=1 -v -run "$pattern" 2>&1 | tee "$log"
  for tn in "${GATES[@]}"; do
    if ! grep -q -- "--- PASS: $tn" "$log"; then
      rm -f "$log"
      fail "$tn 이(가) 실행·통과되지 않았습니다(-run 미매칭 또는 SKIP). 테스트 이름과 GATES 목록을 맞추세요."
      return 1
    fi
  done
  rm -f "$log"
  echo "경계 게이트 ${#GATES[@]}종 전부 실행·통과"
}

# 의존성 최소성 — 이 모듈은 확장 개발자에게 **가벼운 것**이 존재 이유다.
# ⚠ `go list -m all` 을 쓰지 않는다. 의존의 테스트 의존(gopkg.in/check.v1)까지 섞여 나와 소비자가 실제로
#   컴파일하는 것과 다르다. `-deps` + `.Module` 로 「실제로 빌드되는 패키지가 속한 모듈」만 본다
#   (표준 라이브러리는 .Module 이 비어 있어 자연히 빠진다).
deps_check() {
  local mods unexpected
  mods=$(go list -deps -f '{{if .Module}}{{.Module.Path}}{{end}}' ./extension/... | sort -u | sed '/^$/d')
  echo "빌드에 실제로 들어오는 모듈:"
  echo "$mods"
  unexpected=$(echo "$mods" | grep -vE '^(github\.com/heartblast/ableops-sdk|gopkg\.in/yaml\.v3)$' || true)
  if [ -n "$unexpected" ]; then
    echo "$unexpected"
    fail "허용되지 않은 의존이 빌드에 들어옵니다."
    return 1
  fi
  echo "의존성 최소성 통과(gopkg.in/yaml.v3 하나뿐)"
}

# CLAUDE.md 는 매 세션 자동 로딩된다 — 커지면 모든 세션이 그만큼 느려진다. 상세는 .claude/context/ 로.
claude_budget() {
  local limit=8192 size
  [ -f CLAUDE.md ] || { fail "CLAUDE.md 가 없습니다."; return 1; }
  size=$(wc -c < CLAUDE.md | tr -d ' ')
  echo "CLAUDE.md ${size}B / ${limit}B"
  [ "$size" -le "$limit" ] || { fail "CLAUDE.md 가 예산(${limit}B)을 넘었습니다. 상세를 .claude/context/ 로 옮기세요."; return 1; }
}

# 릴리스 계약 — 상수·CHANGELOG·태그 셋이 같은 값이어야 한다.
# TestSDKVersionMatchesChangelog 는 앞의 둘만 보므로 태그는 여기서 확인한다.
release_versions() {
  local const cl tag
  const=$(sed -n 's/^const SDKVersion = "\(.*\)"/\1/p' extension/v1/compat.go)
  cl=$(grep -m1 -E '^##[[:space:]]+v[0-9]+\.[0-9]+\.[0-9]+' CHANGELOG.md | sed -E 's/^##[[:space:]]+v([0-9.]+).*/\1/')
  echo "SDKVersion=${const:-?}  CHANGELOG=${cl:-?}"
  if [ -z "$const" ] || [ "$const" != "$cl" ]; then
    fail "SDKVersion(compat.go)과 CHANGELOG 최신 항목이 다릅니다. 릴리스 때 셋(상수·CHANGELOG·태그)을 함께 올립니다."
    return 1
  fi
  tag=$(git tag --points-at HEAD 2>/dev/null | grep -E '^v[0-9]' | head -1 || true)
  if [ -n "$tag" ]; then
    [ "$tag" = "v$const" ] || { fail "HEAD 태그($tag)가 SDKVersion(v$const)과 다릅니다."; return 1; }
    echo "태그 $tag 일치"
  else
    echo "HEAD 에 v* 태그 없음 — 태그를 붙일 때 v$const 를 씁니다"
  fi
}

# go mod tidy -diff 는 파일을 바꾸지 않고 diff 만 낸다(변화가 있으면 non-zero).
mod_tidy_check() {
  go mod tidy -diff && echo "go mod tidy 무변경"
}

worktree_clean() {
  local s
  s=$(git status --porcelain)
  [ -z "$s" ] && { echo "작업트리 clean"; return 0; }
  echo "$s"
  fail "커밋되지 않은 변경이 있습니다. 릴리스는 clean 트리에서 합니다."
}

# ── 수준별 실행 ─────────────────────────────────────────────────────────────

run_full() {
  step "gofmt" gofmt_check
  step "go build" go build ./...
  step "go vet" go vet ./...
  step "go test(전체 · 외부 consumer 포함)" go test ./...
  step "경계 게이트 이름 확인(${#GATES[@]}종)" gates_check
  step "예제 확장 빌드" go build ./examples/...
  step "의존성 최소성" deps_check
  step "CLAUDE.md 예산" claude_budget
}

case "$LEVEL" in
  fast)
    step "gofmt" gofmt_check
    step "go build" go build ./...
    step "go vet" go vet ./...
    step "go test -short" go test -short ./...
    ;;
  full)
    run_full
    ;;
  release)
    run_full
    step "SDKVersion · CHANGELOG · 태그 일치" release_versions
    step "go mod tidy 무변경" mod_tidy_check
    step "작업트리 clean" worktree_clean
    ;;
esac

echo ""
echo "══ verify.sh $LEVEL 결과 ══════════════════════════════"
for r in "${RESULTS[@]}"; do echo "  $r"; done
if [ "$FAILED" -ne 0 ]; then
  echo "  → 실패한 단계가 있습니다. 통과로 보고하지 마세요."
  exit 1
fi
echo "  → 전부 통과"
