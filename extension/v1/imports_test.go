package extensionv1_test

// SDK 의존성 철칙을 컴파일러 대신 강제하는 가드 테스트.
//
// SDK 가 Core 의 `internal/` 을 import 하면 외부 저장소의 Extension 은 이 SDK 를 컴파일조차
// 할 수 없다(Go 의 internal 패키지 규칙). 그런데 **같은 모듈 안에서는 정상 빌드되므로**
// 실수를 컴파일러가 잡아주지 못한다 — 그래서 AST 로 직접 검사한다.
//
// 저장소를 분리한 뒤에는 Core 를 import 하면 컴파일 자체가 실패하지만, 이 게이트는 그대로 둔다:
// 분리 이전 상태로 되돌아가거나 누군가 Core 를 require 하는 순간 다시 조용해지기 때문이다.

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// allowedExternalImports 는 SDK 가 허용하는 외부(비표준 라이브러리) 의존이다.
// 여기에 추가하려면 "외부 Extension 이 이 의존까지 함께 받아도 되는가"를 먼저 판단해야 한다.
var allowedExternalImports = map[string]bool{
	"gopkg.in/yaml.v3": true,
}

// sdkSelfImportPrefix 는 SDK 트리 **내부**의 패키지 경로다(예: extension/v1/extserver 가
// 상위 extension/v1 을 import 하는 경우).
//
// 모듈 경로가 github.com/... 으로 시작하기 때문에 아래의 "첫 경로 요소에 점이 있으면 외부 모듈"
// 판정에 걸리지만, 이것은 외부 의존이 아니라 **같은 SDK 안의 참조**다. 외부 Extension 이
// 이 SDK 하나만 받으면 되는 자족성은 그대로 유지되므로 검사 대상에서 제외한다.
const sdkSelfImportPrefix = sdkModulePath + "/"

// walkSDKImports 는 SDK 모듈 하위 모든 .go 파일의 import 경로를 순회한다.
func walkSDKImports(t *testing.T, visit func(file, importPath string)) int {
	t.Helper()
	root := sdkModuleRoot(t)
	scanned := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Fatalf("%s 파싱 실패: %v", path, perr)
		}
		scanned++
		for _, imp := range f.Imports {
			p, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil {
				t.Fatalf("%s 의 import 경로 해석 실패: %v", path, uerr)
			}
			visit(path, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("SDK 디렉터리 순회 실패: %v", err)
	}
	return scanned
}

// TestSDKDoesNotImportCore 는 SDK 하위 어떤 파일도 Core 제품 모듈을 import 하지 않음을 강제한다.
//
// `internal/` 뿐 아니라 **Core 모듈 전체**가 대상이다(요청 §2 의 의존 방향):
//
//	외부 Extension → ableops-sdk ← ableops-kafka(Core)
//
// SDK 가 Core 의 공개 패키지를 하나라도 import 하면 화살표가 순환이 되고,
// 확장 개발자의 모듈 그래프에 Core 전체가 다시 실려 나간다.
func TestSDKDoesNotImportCore(t *testing.T) {
	scanned := walkSDKImports(t, func(file, imp string) {
		for _, core := range coreModulePaths {
			if imp == core || strings.HasPrefix(imp, core+"/") {
				t.Errorf("SDK 가 Core 를 import 했다: %s → %q", file, imp)
			}
		}
	})
	if scanned == 0 {
		t.Fatal("검사한 .go 파일이 0개다(테스트가 실제로 아무것도 검증하지 않았다)")
	}
}

// TestSDKDoesNotImportInternal 은 어떤 모듈의 것이든 `internal/` 패키지를 import 하지 않음을 강제한다.
//
// Core 경로 목록에 없는 제3의 모듈이 들어와도 잡히도록 경로 요소로 판정한다 —
// 외부 소비자에게는 "internal 이라서 컴파일 불가"라는 결과가 동일하기 때문이다.
func TestSDKDoesNotImportInternal(t *testing.T) {
	walkSDKImports(t, func(file, imp string) {
		for _, seg := range strings.Split(imp, "/") {
			if seg == "internal" {
				t.Errorf("SDK 가 internal 패키지를 import 했다(외부 소비자는 컴파일할 수 없다): %s → %q", file, imp)
				return
			}
		}
	})
}

// TestSDKHasNoUnexpectedDependency 는 표준 라이브러리 + 허용 목록 외 의존이 들어오는 것을 막는다.
// 외부 Extension 이 이 SDK 하나만 받으면 되도록 자족성을 유지하기 위함이다.
func TestSDKHasNoUnexpectedDependency(t *testing.T) {
	walkSDKImports(t, func(file, imp string) {
		if allowedExternalImports[imp] {
			return
		}
		if strings.HasPrefix(imp, sdkSelfImportPrefix) {
			return // SDK 트리 내부 참조(위 sdkSelfImportPrefix 주석 참조)
		}
		// 첫 경로 요소에 점(.)이 있으면 외부 모듈이다(표준 라이브러리에는 없다).
		first, _, _ := strings.Cut(imp, "/")
		if strings.Contains(first, ".") {
			t.Errorf("허용되지 않은 외부 의존이다: %s → %q", file, imp)
		}
	})
}

// TestSDKDoesNotImportChi 는 SDK 가 라우터 구현에 묶이지 않음을 강제한다(extension.go 주석 참조).
func TestSDKDoesNotImportChi(t *testing.T) {
	walkSDKImports(t, func(file, imp string) {
		if strings.Contains(imp, "go-chi/chi") {
			t.Errorf("SDK 가 chi 를 import 했다(net/http 만 사용해야 한다): %s → %q", file, imp)
		}
	})
}
