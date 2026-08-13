package extensionv1

// SDK 의존성 철칙을 컴파일러 대신 강제하는 가드 테스트.
//
// sdk/ 가 internal/ 을 import 하면 외부 저장소의 Extension 은 이 SDK 를 컴파일조차 할 수 없다
// (Go 의 internal 패키지 규칙). 그런데 같은 모듈 안에서는 import 가 **정상 빌드되므로**
// 실수를 컴파일러가 잡아주지 못한다 — 그래서 AST 로 직접 검사한다.

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// bannedImportPrefix 는 SDK 가 절대 import 하면 안 되는 경로다.
const bannedImportPrefix = "github.com/heartblast/kafka-control-portal/internal"

// allowedExternalImports 는 SDK 가 허용하는 외부(비표준 라이브러리) 의존이다.
// 여기에 추가하려면 "외부 Extension 이 이 의존까지 함께 받아도 되는가"를 먼저 판단해야 한다.
var allowedExternalImports = map[string]bool{
	"gopkg.in/yaml.v3": true,
}

// sdkRoot 는 sdk/ 디렉터리 경로를 찾는다(테스트는 패키지 디렉터리에서 실행된다).
func sdkRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("작업 디렉터리 조회 실패: %v", err)
	}
	for i := 0; i < 8; i++ {
		if filepath.Base(dir) == "sdk" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("sdk 디렉터리를 찾지 못했다(시작: %s)", dir)
	return ""
}

// walkSDKImports 는 sdk/ 하위 모든 .go 파일의 import 경로를 순회한다.
func walkSDKImports(t *testing.T, visit func(file, importPath string)) int {
	t.Helper()
	root := sdkRoot(t)
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
		t.Fatalf("sdk 디렉터리 순회 실패: %v", err)
	}
	return scanned
}

// TestSDKDoesNotImportInternal 은 sdk/ 하위 어떤 파일도 internal/ 을 import 하지 않음을 강제한다.
//
// sdk 내부 패키지끼리의 전이 의존도 함께 검사한다(sdk 전체를 훑기 때문).
// 외부 모듈(yaml.v3 등)은 모듈 경계상 이 저장소의 internal 을 import 할 수 없으므로 검사 대상이 아니다.
func TestSDKDoesNotImportInternal(t *testing.T) {
	scanned := walkSDKImports(t, func(file, imp string) {
		if imp == bannedImportPrefix || strings.HasPrefix(imp, bannedImportPrefix+"/") {
			t.Errorf("SDK 가 internal 을 import 했다: %s → %q", file, imp)
		}
	})
	if scanned == 0 {
		t.Fatal("검사한 .go 파일이 0개다(테스트가 실제로 아무것도 검증하지 않았다)")
	}
}

// TestSDKHasNoUnexpectedDependency 는 표준 라이브러리 + 허용 목록 외 의존이 들어오는 것을 막는다.
// 외부 Extension 이 이 SDK 하나만 받으면 되도록 자족성을 유지하기 위함이다.
func TestSDKHasNoUnexpectedDependency(t *testing.T) {
	walkSDKImports(t, func(file, imp string) {
		if allowedExternalImports[imp] {
			return
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
