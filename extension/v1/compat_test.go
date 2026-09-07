package extensionv1_test

// SDKVersion 이 배포물(태그·CHANGELOG)과 어긋나는 것을 막는다.
//
// 상수 하나가 손으로 유지되는 순간 반드시 드리프트한다 — 릴리스 때 태그만 붙이고 상수는
// 그대로 두면, Core 는 "SDK 1.0.0 을 쓰는 중"이라고 보고하면서 실제로는 1.1.0 을 쓴다.
// 그 상태에서 지원 문의가 오면 아무도 원인을 찾지 못한다.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// changelogHeading 은 CHANGELOG 의 릴리스 제목이다: `## v1.0.0 — 2026-09-07`
var changelogHeading = regexp.MustCompile(`(?m)^##\s+v(\d+\.\d+\.\d+)\b`)

// TestSDKVersionMatchesChangelog 는 SDKVersion 이 CHANGELOG 최신 항목과 같은지 확인한다.
func TestSDKVersionMatchesChangelog(t *testing.T) {
	path := filepath.Join(sdkModuleRoot(t), "CHANGELOG.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("CHANGELOG.md 를 읽지 못했다(%s): %v", path, err)
	}
	m := changelogHeading.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("CHANGELOG.md 에서 릴리스 제목(`## vX.Y.Z`)을 찾지 못했다")
	}
	if m[1] != extv1.SDKVersion {
		t.Fatalf("SDKVersion(%q)과 CHANGELOG 최신 항목(%q)이 다르다 — 릴리스 때 셋(상수·CHANGELOG·태그)을 함께 올린다",
			extv1.SDKVersion, m[1])
	}
}

// TestSDKVersionIsSemver 는 SDKVersion 이 SDK 자신의 파서로 해석되는지 확인한다.
// 자기 파서로도 못 읽는 버전 문자열은 Core 의 호환 판정에서 그대로 실패한다.
func TestSDKVersionIsSemver(t *testing.T) {
	if strings.HasPrefix(extv1.SDKVersion, "v") {
		t.Fatalf("SDKVersion 에 v 접두를 넣지 않는다(태그가 v 를 붙인다): %q", extv1.SDKVersion)
	}
	if _, _, _, err := extv1.ParseSemver(extv1.SDKVersion); err != nil {
		t.Fatalf("SDKVersion 이 semver 가 아니다: %q (%v)", extv1.SDKVersion, err)
	}
}

// TestAPIVersionMatchesModuleMajor 는 API 버전 세그먼트가 패키지 경로와 일치하는지 고정한다.
//
// `extension/v1` 패키지가 `ableops.io/extension/v2` 를 선언하면 Core 의 apiVersion 판정과
// 소비자의 import 경로가 어긋난다 — 둘 다 빌드는 통과한다.
func TestAPIVersionMatchesModuleMajor(t *testing.T) {
	const want = "ableops.io/extension/v1"
	if extv1.APIVersion != want {
		t.Fatalf("APIVersion 이 %q 가 아니다: %q — v1 패키지가 다른 API 버전을 말하고 있다", want, extv1.APIVersion)
	}
}
