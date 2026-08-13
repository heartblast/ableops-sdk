package extensionv1

// semver 파싱과 버전 제약(range) 판정 — 외부 의존 없이 표준 라이브러리만 사용한다.
//
// ⚠ 개발 빌드 버전("dev"/"unknown"/"")의 처리는 **Core 정책**이며 이 파일이 결정하지 않는다.
// ParseSemver 는 그런 값에 명확한 에러를 돌려주므로, **호출자가 개발 빌드를 먼저 분기**한 뒤
// 이 함수들을 불러야 한다(Core 는 internal/extensionhost 에서 "dev 는 모든 제약 통과"로 처리한다).
// SDK 가 임의로 "dev = 무조건 통과"로 정하면 배포본에서도 같은 구멍이 생긴다.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseSemver 는 "1.2.3" 형식의 semver 를 major/minor/patch 로 분해한다.
//
// 선행 "v" 접두는 허용한다. prerelease("-beta.1")·build("+20260813") 메타데이터는 **허용하되 무시**한다.
// major/minor/patch 3요소가 모두 있어야 하며 각 요소는 음수가 아닌 10진 정수여야 한다.
// 제약 문자열 안의 부분 버전(">=1.3" 의 "1.3")은 SatisfiesRange 가 별도로 관대하게 처리한다.
func ParseSemver(s string) (major, minor, patch int, err error) {
	core, err := stripSemverMeta(s)
	if err != nil {
		return 0, 0, 0, err
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("semver 는 major.minor.patch 3요소여야 합니다: %q", s)
	}
	nums, err := parseVersionParts(parts, s)
	if err != nil {
		return 0, 0, 0, err
	}
	return nums[0], nums[1], nums[2], nil
}

// stripSemverMeta 는 "v" 접두와 prerelease/build 메타데이터를 떼고 숫자 본체만 남긴다.
func stripSemverMeta(s string) (string, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return "", errors.New("버전이 비어 있습니다")
	}
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("버전에 숫자 부분이 없습니다: %q", s)
	}
	return v, nil
}

// parseVersionParts 는 점으로 나뉜 숫자 요소들을 정수로 변환한다.
func parseVersionParts(parts []string, orig string) ([]int, error) {
	out := make([]int, len(parts))
	for i, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("버전 요소가 비어 있습니다: %q", orig)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("버전 요소가 숫자가 아닙니다: %q (요소 %q)", orig, p)
		}
		out[i] = n
	}
	return out, nil
}

// parseVersionLoose 는 "1", "1.3", "1.3.0" 을 모두 받아 [major, minor, patch] 로 채운다.
// 빠진 요소는 0 으로 본다(">=1.3" == ">=1.3.0"). 제약 판정 전용이다.
func parseVersionLoose(s string) ([3]int, error) {
	var v [3]int
	core, err := stripSemverMeta(s)
	if err != nil {
		return v, err
	}
	parts := strings.Split(core, ".")
	if len(parts) > 3 {
		return v, fmt.Errorf("버전 요소가 너무 많습니다: %q", s)
	}
	nums, err := parseVersionParts(parts, s)
	if err != nil {
		return v, err
	}
	copy(v[:], nums)
	return v, nil
}

// compareVersion 은 a 와 b 를 비교한다(-1: a<b, 0: a==b, 1: a>b).
func compareVersion(a, b [3]int) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

// SatisfiesRange 는 version 이 constraint 를 만족하는지 판정한다.
//
// constraint 는 공백(또는 쉼표)으로 구분된 토큰의 **AND 결합**이다: ">=1.3.0 <2.0.0".
// 토큰 형식: ">=1.3.0" ">1.3.0" "<=2.0.0" "<2.0.0" "=1.3.0" 그리고 연산자 없는 맨 버전("1.3.0" = 동등).
// **빈 constraint(공백만 포함)는 제한 없음이므로 무조건 true** 다.
// 지원하지 않는 연산자(^, ~ 등)나 파싱 불가한 버전은 조용히 통과시키지 않고 에러를 돌려준다.
func SatisfiesRange(version, constraint string) (bool, error) {
	c := strings.TrimSpace(constraint)
	if c == "" {
		return true, nil
	}
	target, err := parseVersionLoose(version)
	if err != nil {
		return false, fmt.Errorf("비교 대상 버전을 해석할 수 없습니다: %w", err)
	}
	for _, token := range strings.Fields(strings.ReplaceAll(c, ",", " ")) {
		ok, err := satisfiesToken(target, token)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// satisfiesToken 은 제약 토큰 1개를 판정한다.
func satisfiesToken(target [3]int, token string) (bool, error) {
	op, rest := splitConstraintOp(token)
	if rest == "" {
		return false, fmt.Errorf("버전 제약에 버전이 없습니다: %q", token)
	}
	if op == "" {
		op = "="
	}
	bound, err := parseVersionLoose(rest)
	if err != nil {
		return false, fmt.Errorf("버전 제약 %q 해석 실패: %w", token, err)
	}
	cmp := compareVersion(target, bound)
	switch op {
	case ">=":
		return cmp >= 0, nil
	case ">":
		return cmp > 0, nil
	case "<=":
		return cmp <= 0, nil
	case "<":
		return cmp < 0, nil
	case "=", "==":
		return cmp == 0, nil
	default:
		return false, fmt.Errorf("지원하지 않는 버전 제약 연산자입니다: %q (사용 가능: >= > <= < =)", token)
	}
}

// splitConstraintOp 는 토큰을 연산자와 버전 문자열로 나눈다.
// 연산자 문자로 시작하면 그 구간 전부를 연산자로 잡아 "^1.2.3" 같은 미지원 표기가
// 버전 파싱 오류가 아니라 "미지원 연산자" 오류로 보고되게 한다.
func splitConstraintOp(token string) (op, rest string) {
	i := 0
	for i < len(token) && strings.ContainsRune("<>=!~^", rune(token[i])) {
		i++
	}
	return token[:i], strings.TrimSpace(token[i:])
}
