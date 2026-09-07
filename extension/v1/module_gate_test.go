package extensionv1_test

// SDK **모듈 경계** 게이트 — 이 분리가 다시 무너지는 것을 막는다.
//
// # 무엇을 지키는가
//
// SDK 는 Core 에서 떼어낸 독립 모듈(`github.com/heartblast/ableops-sdk`)이고,
// 독립 저장소의 루트가 된다. 그 이유는 하나다 —
// **외부 확장 개발자가 SDK 만 쓸 때 Core/CLI 의 의존성이 따라가지 않게** 하기 위해서다.
// Core 모듈에는 Kafka 클라이언트·DB 드라이버 3종·Prometheus·TUI 라이브러리가 있다.
//
// 그런데 이 분리는 **한 줄로 무너진다**: 누군가 SDK 안에서 그 패키지들을 import 하고
// `go mod tidy` 를 돌리면 go.mod 에 요구가 추가되고, 그 순간 외부 소비자의 모듈 그래프에
// 다시 실려 나간다. 컴파일도 테스트도 통과하므로 아무도 알아채지 못한다.
//
// ⚠ 이 테스트는 **go.mod 파일 자체**를 읽는다. import 스캔(imports_test.go)만으로는
// 부족하다 — go.mod 의 require 는 import 없이도 손으로 추가될 수 있고,
// 모듈 그래프에 실리는 것은 import 가 아니라 **require** 이기 때문이다.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ModulePath 는 이 SDK 의 공개 모듈 경로다.
//
// ⚠ 바꾸면 외부 Extension 의 import 문이 **전부** 깨진다. 바꿔야 한다면 그것은
// 새 major(`/v2`)이지 이 상수의 수정이 아니다.
const sdkModulePath = "github.com/heartblast/ableops-sdk"

// coreModulePaths 는 SDK 가 절대 요구·import 하면 안 되는 **Core 제품 모듈**이다.
//
// 과거 경로(kafka-control-portal)와 현재 제품 경로(ableops-kafka)를 함께 막는다 —
// Core 모듈 경로가 바뀌는 중에도 게이트가 조용히 통과하는 구멍을 만들지 않기 위해서다.
var coreModulePaths = []string{
	"github.com/heartblast/ableops-kafka",
	"github.com/heartblast/kafka-control-portal",
}

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

// TestSDKModuleRequiresStayMinimal 는 go.mod 의 require 목록을 고정한다.
func TestSDKModuleRequiresStayMinimal(t *testing.T) {
	data := readSDKGoMod(t)

	for _, banned := range bannedModulePrefixes {
		if strings.Contains(data, banned) {
			t.Errorf("SDK go.mod 가 %q 를 요구한다 — 이것이 외부 SDK 소비자의 모듈 그래프에 실려 나간다.\n"+
				"  SDK 는 확장 개발자에게 필요한 것만 요구해야 한다. Core/CLI 전용 의존은 Core 모듈에 둔다.", banned)
		}
	}
}

// TestSDKModuleDoesNotRequireCore 는 SDK 가 Core 제품 모듈을 되끌어오지 않음을 고정한다.
//
// ⚠ 이것이 들어오면 분리가 **완전히** 무의미해진다 — Core 를 요구하는 순간 Core 의 모든 요구가
// 소비자의 그래프에 따라 들어오고, 의존 방향(Core → SDK)이 순환이 된다.
func TestSDKModuleDoesNotRequireCore(t *testing.T) {
	data := readSDKGoMod(t)
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "module ") {
			continue
		}
		for _, core := range coreModulePaths {
			if strings.Contains(trimmed, core) {
				t.Fatalf("SDK go.mod 가 Core 모듈(%s)을 요구한다 — 분리가 무의미해진다:\n  %s", core, trimmed)
			}
		}
	}
}

// TestSDKModulePathIsStandalone 는 모듈 경로가 **독립 저장소 경로**인지 확인한다.
//
// Core 하위 경로(.../kafka-control-portal/sdk)로 되돌아가면 SDK 는 다시 Core 저장소의
// 태그에 묶이고, 「SDK 와 Core 를 독립 버전으로 릴리스한다」가 성립하지 않는다.
func TestSDKModulePathIsStandalone(t *testing.T) {
	data := readSDKGoMod(t)
	want := "module " + sdkModulePath
	if !strings.Contains(data, want) {
		t.Fatalf("SDK go.mod 의 module 경로가 %q 가 아니다 — 바꾸면 외부 import 문이 전부 깨진다:\n%s",
			want, data)
	}
	for _, core := range coreModulePaths {
		if strings.Contains(data, "module "+core) {
			t.Fatalf("SDK 모듈 경로가 Core 저장소 하위로 되돌아갔다: %s", core)
		}
	}
}

// sdkModuleRoot 는 이 SDK 모듈의 루트 디렉터리를 찾는다.
//
// ⚠ 디렉터리 **이름**으로 찾지 않는다. 이 트리는 두 곳에서 같은 코드로 돌아야 한다:
//
//	Core 저장소 안(스테이징)   <core>/sdk/extension/v1/...
//	독립 저장소(추출 후)        <ableops-sdk>/extension/v1/...
//
// 이름("sdk")에 의존하면 추출 직후 모든 테스트가 깨지고, 그것을 고치는 수정이
// 「추출은 순수 복사여야 한다」는 이 작업의 전제를 무너뜨린다.
// 그래서 go.mod 의 **module 선언**으로 찾는다 — 두 배치에서 같은 값이다.
func sdkModuleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("호출 위치를 알 수 없다")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 8; i++ {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module "+sdkModulePath) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("SDK 모듈 루트(module %s 를 선언한 go.mod)를 찾지 못했다: %s", sdkModulePath, file)
	return ""
}

// readSDKGoMod 는 SDK 모듈의 go.mod 를 읽는다.
func readSDKGoMod(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sdkModuleRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("SDK go.mod 를 읽지 못했다: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("SDK go.mod 가 비어 있다")
	}
	return string(data)
}
