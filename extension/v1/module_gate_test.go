package extensionv1_test

// SDK **모듈 경계** 게이트 — 이 분리가 다시 무너지는 것을 막는다.
//
// # 무엇을 지키는가
//
// SDK 는 루트에서 떼어낸 독립 모듈이다(sdk/go.mod). 그 이유는 하나다 —
// **외부 확장 개발자가 SDK 만 쓸 때 CLI/Core 의 의존성이 따라가지 않게** 하기 위해서다.
// 루트 모듈에는 Kafka 클라이언트·DB 드라이버 3종·Prometheus 가 있고, 앞으로 TUI 라이브러리
// (Bubble Tea·Bubbles·Lip Gloss 와 그 20여 개 간접 의존)가 들어온다.
//
// 그런데 이 분리는 **한 줄로 무너진다**: 누군가 sdk 안에서 그 패키지들을 import 하고
// `go mod tidy` 를 돌리면 sdk/go.mod 에 요구가 추가되고, 그 순간 외부 소비자의 모듈 그래프에
// 다시 실려 나간다. 컴파일도 테스트도 통과하므로 아무도 알아채지 못한다.
//
// ⚠ 이 테스트는 **sdk/go.mod 파일 자체**를 읽는다. import 스캔(imports_test.go)만으로는
// 부족하다 — go.mod 의 require 는 import 없이도 손으로 추가될 수 있고,
// 모듈 그래프에 실리는 것은 import 가 아니라 **require** 이기 때문이다.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// bannedModulePrefixes 는 SDK 모듈이 절대 요구하면 안 되는 것들이다.
//
// 판정 근거는 「무거워서」가 아니라 「**SDK 를 쓰는 사람에게 필요 없어서**」다.
// 확장 개발자는 Manifest 를 파싱하고 HTTP 핸들러를 등록할 뿐, Kafka 에 붙지도 DB 를 열지도
// 터미널을 그리지도 않는다.
var bannedModulePrefixes = []string{
	// TUI — CLI 계층 전용(220 4단계).
	"github.com/charmbracelet/",
	"github.com/muesli/",
	"github.com/lucasb-eyer/",
	"github.com/mattn/go-runewidth",
	"github.com/rivo/uniseg",
	"github.com/xo/terminfo",
	// Core 런타임 — 서버 전용.
	"github.com/twmb/franz-go",
	"github.com/jackc/pgx",
	"github.com/go-sql-driver/mysql",
	"github.com/sijms/go-ora",
	"github.com/prometheus/",
	"github.com/go-chi/chi",
	"github.com/coreos/go-oidc",
	"github.com/go-ldap/ldap",
}

// TestSDKModuleRequiresStayMinimal 는 sdk/go.mod 의 require 목록을 고정한다.
func TestSDKModuleRequiresStayMinimal(t *testing.T) {
	data := readSDKGoMod(t)

	for _, banned := range bannedModulePrefixes {
		if strings.Contains(data, banned) {
			t.Errorf("sdk/go.mod 가 %q 를 요구한다 — 이것이 외부 SDK 소비자의 모듈 그래프에 실려 나간다.\n"+
				"  SDK 는 확장 개발자에게 필요한 것만 요구해야 한다. CLI/Core 전용 의존은 루트 모듈에 둔다.", banned)
		}
	}
}

// TestSDKModuleDoesNotRequireRoot 는 SDK 가 루트 모듈을 되끌어오지 않음을 고정한다.
//
// ⚠ 이것이 들어오면 분리가 **완전히** 무의미해진다 — 루트를 요구하는 순간 루트의 모든 요구가
// 소비자의 그래프에 따라 들어오기 때문이다.
func TestSDKModuleDoesNotRequireRoot(t *testing.T) {
	data := readSDKGoMod(t)
	// 자기 자신(.../sdk)은 module 선언에만 나온다. 루트 경로가 require 로 나오면 위반이다.
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "module ") {
			continue
		}
		if strings.Contains(trimmed, "github.com/heartblast/kafka-control-portal") &&
			!strings.Contains(trimmed, "kafka-control-portal/sdk") {
			t.Fatalf("sdk/go.mod 가 루트 모듈을 요구한다 — 분리가 무의미해진다:\n  %s", trimmed)
		}
	}
}

// TestSDKModulePathIsNested 는 모듈 경로가 루트의 하위인지 확인한다.
//
// 경로가 이것이어야 **import 문이 바뀌지 않는다** — Go 가 최장 일치 모듈 접두로 해소하므로
// 기존 `.../kafka-control-portal/sdk/extension/v1` 이 그대로 동작한다.
func TestSDKModulePathIsNested(t *testing.T) {
	data := readSDKGoMod(t)
	const want = "module github.com/heartblast/kafka-control-portal/sdk"
	if !strings.Contains(data, want) {
		t.Fatalf("sdk/go.mod 의 module 경로가 %q 가 아니다 — 바꾸면 외부 import 문이 전부 깨진다:\n%s",
			want, data)
	}
}

// readSDKGoMod 는 sdk/go.mod 를 읽는다.
func readSDKGoMod(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("호출 위치를 알 수 없다")
	}
	// <root>/sdk/extension/v1/module_gate_test.go → 2단계 위가 <root>/sdk
	sdkRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	data, err := os.ReadFile(filepath.Join(sdkRoot, "go.mod"))
	if err != nil {
		t.Fatalf("sdk/go.mod 를 읽지 못했다: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("sdk/go.mod 가 비어 있다")
	}
	return string(data)
}
