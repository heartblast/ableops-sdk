package extensionv1_test

// SDK v1 **공개 API 스냅샷** — 우발적인 파괴적 변경을 컴파일러 대신 잡는다(요구 §12).
//
// # 왜 필요한가
//
// SDK 가 외부 개발자에게 쓰이기 시작하면 공개 심볼을 바꾸는 비용이 폭증한다. 그런데 파괴적
// 변경 대부분은 **테스트를 통과한다**: 인터페이스에 메서드를 하나 더하면 이 저장소는 멀쩡히
// 빌드되지만 외부의 모든 구현체가 깨지고, 필드 이름을 다듬으면 외부 코드만 깨진다.
// 그래서 공개 심볼 목록 자체를 골든 파일로 고정한다.
//
// # 무엇을 잡는가
//
//	타입·필드·메서드·함수·상수의 추가/삭제/시그니처 변경
//	인터페이스 메서드 집합 변경(외부 구현체를 깨뜨리는 대표적 사례)
//
// # 갱신 방법
//
//	UPDATE_SDK_API_GOLDEN=1 go test ./sdk/extension/v1 -run TestPublicAPISnapshot
//
// ⚠ **diff 를 반드시 눈으로 확인한다.** 줄이 사라졌다면 그것은 파괴적 변경이며,
// v1 을 유지한 채로는 허용되지 않는다(문서 docs/reference_docs/extension-sdk/버전호환정책.md).
// 줄이 추가되기만 했다면 하위호환 확장이다 — 단, **인터페이스 안에서의 추가**는 예외적으로
// 파괴적이다(외부 구현체가 새 메서드를 갖고 있을 리 없다).

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// goldenPath 는 공개 API 스냅샷 파일이다.
const goldenPath = "testdata/api-v1.golden"

// snapshotPackages 는 스냅샷 대상 패키지다(경로는 SDK 모듈 루트 기준 상대).
//
// testkit 을 포함하는 이유: 외부 개발자의 테스트 코드가 이 API 에 직접 의존하므로,
// 조용히 바뀌면 그들의 테스트가 깨진다.
var snapshotPackages = []string{
	"extension/v1",
	"extension/v1/extserver",
	"extension/v1/testkit",
}

// TestPublicAPISnapshot 은 공개 심볼 목록이 골든과 일치하는지 확인한다.
func TestPublicAPISnapshot(t *testing.T) {
	root := sdkModuleRoot(t)
	var lines []string
	for _, pkg := range snapshotPackages {
		dir := filepath.Join(root, filepath.FromSlash(pkg))
		lines = append(lines, renderPackageAPI(t, pkg, dir)...)
	}
	got := strings.Join(lines, "\n") + "\n"

	goldenFile := filepath.Join(root, "extension", "v1", filepath.FromSlash(goldenPath))
	if os.Getenv("UPDATE_SDK_API_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenFile), 0o750); err != nil {
			t.Fatalf("골든 디렉터리 생성 실패: %v", err)
		}
		if err := os.WriteFile(goldenFile, []byte(got), 0o600); err != nil {
			t.Fatalf("골든 갱신 실패: %v", err)
		}
		t.Logf("공개 API 골든을 갱신했다: %s (diff 를 반드시 확인할 것)", goldenFile)
		return
	}

	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("골든 파일을 읽을 수 없다(%s): %v\nUPDATE_SDK_API_GOLDEN=1 로 생성하세요", goldenFile, err)
	}
	if string(want) == got {
		return
	}
	t.Errorf("SDK v1 공개 API 가 골든과 다르다.\n%s\n\n"+
		"의도한 변경이면 UPDATE_SDK_API_GOLDEN=1 로 갱신하고 diff 를 확인하세요.\n"+
		"⚠ 줄이 **사라졌거나** 인터페이스에 메서드가 **추가**되었다면 파괴적 변경입니다(v1 에서 허용되지 않음).",
		unifiedish(string(want), got))
}

// renderPackageAPI 는 패키지의 공개 심볼을 안정적인 텍스트로 만든다.
func renderPackageAPI(t *testing.T, pkgPath, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		// 테스트 파일은 공개 API 가 아니다.
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("%s 파싱 실패: %v", dir, err)
	}
	var out []string
	for _, pkg := range pkgs {
		var items []string
		for _, file := range pkg.Files {
			items = append(items, renderFileAPI(fset, file)...)
		}
		sort.Strings(items)
		out = append(out, fmt.Sprintf("# package %s (%s)", pkg.Name, pkgPath))
		out = append(out, items...)
		out = append(out, "")
	}
	return out
}

// renderFileAPI 는 파일 하나의 공개 선언을 렌더링한다.
func renderFileAPI(fset *token.FileSet, file *ast.File) []string {
	var out []string
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			if d.Recv == nil {
				out = append(out, "func "+d.Name.Name+renderFuncType(fset, d.Type))
				continue
			}
			recv := renderExpr(fset, d.Recv.List[0].Type)
			if !exportedReceiver(recv) {
				continue
			}
			out = append(out, "method ("+recv+") "+d.Name.Name+renderFuncType(fset, d.Type))
		case *ast.GenDecl:
			out = append(out, renderGenDecl(fset, d)...)
		}
	}
	return out
}

// exportedReceiver 는 리시버 타입이 공개 타입인지 본다(*Foo · Foo).
func exportedReceiver(recv string) bool {
	name := strings.TrimPrefix(recv, "*")
	if name == "" {
		return false
	}
	return ast.IsExported(name)
}

// renderGenDecl 은 type/const/var 선언을 렌더링한다.
func renderGenDecl(fset *token.FileSet, d *ast.GenDecl) []string {
	var out []string
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			out = append(out, renderTypeSpec(fset, s)...)
		case *ast.ValueSpec:
			kind := "var"
			if d.Tok == token.CONST {
				kind = "const"
			}
			for _, name := range s.Names {
				if !name.IsExported() {
					continue
				}
				line := kind + " " + name.Name
				if s.Type != nil {
					line += " " + renderExpr(fset, s.Type)
				}
				out = append(out, line)
			}
		}
	}
	return out
}

// renderTypeSpec 은 타입 선언을 렌더링한다(구조체 필드·인터페이스 메서드까지).
func renderTypeSpec(fset *token.FileSet, s *ast.TypeSpec) []string {
	name := s.Name.Name
	switch t := s.Type.(type) {
	case *ast.StructType:
		out := []string{"type " + name + " struct"}
		for _, f := range structFields(fset, t) {
			out = append(out, "  field "+name+"."+f)
		}
		return out
	case *ast.InterfaceType:
		out := []string{"type " + name + " interface"}
		for _, m := range interfaceMethods(fset, t) {
			// ⚠ 인터페이스 메서드 **추가**는 외부 구현체를 깨뜨리는 파괴적 변경이다.
			out = append(out, "  imethod "+name+"."+m)
		}
		return out
	default:
		if s.Assign.IsValid() {
			return []string{"type " + name + " = " + renderExpr(fset, s.Type)}
		}
		return []string{"type " + name + " " + renderExpr(fset, s.Type)}
	}
}

// structFields 는 공개 필드 목록을 만든다(비공개 필드는 계약이 아니다).
func structFields(fset *token.FileSet, t *ast.StructType) []string {
	var out []string
	if t.Fields == nil {
		return out
	}
	for _, f := range t.Fields.List {
		typ := renderExpr(fset, f.Type)
		tag := ""
		if f.Tag != nil {
			// JSON/YAML 태그도 계약이다(wire format 이 그것으로 결정된다).
			tag = " " + f.Tag.Value
		}
		if len(f.Names) == 0 { // 임베딩
			out = append(out, typ+tag)
			continue
		}
		for _, n := range f.Names {
			if !n.IsExported() {
				continue
			}
			out = append(out, n.Name+" "+typ+tag)
		}
	}
	sort.Strings(out)
	return out
}

// interfaceMethods 는 인터페이스 메서드 집합을 만든다.
func interfaceMethods(fset *token.FileSet, t *ast.InterfaceType) []string {
	var out []string
	if t.Methods == nil {
		return out
	}
	for _, m := range t.Methods.List {
		ft, ok := m.Type.(*ast.FuncType)
		if !ok { // 임베딩된 인터페이스
			out = append(out, renderExpr(fset, m.Type))
			continue
		}
		for _, n := range m.Names {
			out = append(out, n.Name+renderFuncType(fset, ft))
		}
	}
	sort.Strings(out)
	return out
}

// renderFuncType 은 함수 시그니처를 문자열로 만든다(파라미터 이름은 제외 — 계약이 아니다).
func renderFuncType(fset *token.FileSet, ft *ast.FuncType) string {
	var b strings.Builder
	if ft.TypeParams != nil {
		b.WriteString("[")
		b.WriteString(strings.Join(fieldTypes(fset, ft.TypeParams, true), ", "))
		b.WriteString("]")
	}
	b.WriteString("(")
	b.WriteString(strings.Join(fieldTypes(fset, ft.Params, false), ", "))
	b.WriteString(")")
	if ft.Results != nil && len(ft.Results.List) > 0 {
		res := fieldTypes(fset, ft.Results, false)
		if len(res) == 1 {
			b.WriteString(" " + res[0])
		} else {
			b.WriteString(" (" + strings.Join(res, ", ") + ")")
		}
	}
	return b.String()
}

// fieldTypes 는 파라미터/결과 타입 목록을 만든다.
func fieldTypes(fset *token.FileSet, fl *ast.FieldList, withNames bool) []string {
	var out []string
	if fl == nil {
		return out
	}
	for _, f := range fl.List {
		typ := renderExpr(fset, f.Type)
		n := len(f.Names)
		if n == 0 {
			out = append(out, typ)
			continue
		}
		for _, name := range f.Names {
			if withNames {
				out = append(out, name.Name+" "+typ)
				continue
			}
			out = append(out, typ)
		}
	}
	return out
}

// renderExpr 는 타입 표현식을 원본 그대로 문자열로 만든다.
func renderExpr(fset *token.FileSet, e ast.Expr) string {
	if e == nil {
		return ""
	}
	start := fset.Position(e.Pos())
	end := fset.Position(e.End())
	if start.Filename != end.Filename || start.Offset >= end.Offset {
		return fmt.Sprintf("%T", e)
	}
	data, err := os.ReadFile(start.Filename)
	if err != nil || end.Offset > len(data) {
		return fmt.Sprintf("%T", e)
	}
	return normalizeSpace(string(data[start.Offset:end.Offset]))
}

// normalizeSpace 는 줄바꿈·연속 공백을 하나로 줄인다(포맷 변화가 골든을 흔들지 않게).
func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// unifiedish 는 사람이 읽을 수 있는 간단한 차이 목록을 만든다.
func unifiedish(want, got string) string {
	wantSet := map[string]bool{}
	for _, l := range strings.Split(want, "\n") {
		wantSet[l] = true
	}
	gotSet := map[string]bool{}
	for _, l := range strings.Split(got, "\n") {
		gotSet[l] = true
	}
	var b strings.Builder
	for _, l := range strings.Split(want, "\n") {
		if l != "" && !gotSet[l] {
			b.WriteString("- " + l + "\n")
		}
	}
	for _, l := range strings.Split(got, "\n") {
		if l != "" && !wantSet[l] {
			b.WriteString("+ " + l + "\n")
		}
	}
	if b.Len() == 0 {
		return "(줄 집합은 같으나 순서가 다르다)"
	}
	return b.String()
}
