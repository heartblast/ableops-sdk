package extensionv1_test

// 외부 Consumer 컴파일 검증 — **저장소 밖 모듈**에서 SDK 를 쓸 수 있는지 확인한다(요구 §11).
//
// # 왜 필요한가
//
// 같은 모듈 안에서 하는 테스트로는 잡히지 않는 실패가 있다:
//
//   - SDK 가 internal/ 을 import 하면 같은 모듈에서는 **정상 빌드된다**(외부에서만 깨진다).
//   - SDK 가 무거운 Core 의존을 끌고 들어와도 같은 모듈에서는 티가 나지 않는다.
//   - Manifest·testkit·extserver 를 함께 쓰는 실제 사용 형태가 컴파일되는지 알 수 없다.
//
// 그래서 testdata/sdk-consumer 를 **별도 go.mod 를 가진 모듈**로 두고, 임시 디렉터리에 복사해
// 실제로 `go build` 를 돌린다. replace 로 이 저장소를 가리키므로 로컬 SDK 변경이 즉시 반영된다.
//
// # 네트워크를 쓰지 않는다
//
// GOPROXY=off 로 모듈 캐시만 사용한다. 저장소가 이미 빌드된 환경이면 필요한 의존
// (gopkg.in/yaml.v3)은 캐시에 있다. 캐시에 없어 실패하면 그 사실을 그대로 드러낸다 —
// 조용히 건너뛰면 이 테스트는 있으나 마나 한 것이 된다.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestExternalConsumerCompiles 는 외부 모듈에서 SDK 를 import 해 빌드되는지 확인한다.
func TestExternalConsumerCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: 외부 모듈 컴파일 검증을 건너뛴다")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go 실행 파일을 찾을 수 없다: %v", err)
	}

	repoRoot := repoRootDir(t)
	fixture := filepath.Join(repoRoot, "sdk", "extension", "v1", "testdata", "sdk-consumer")
	work := t.TempDir()
	copyTree(t, fixture, work)

	// go.mod 를 만든다(replace 대상은 이 저장소의 절대 경로).
	tmpl, err := os.ReadFile(filepath.Join(work, "go.mod.tmpl"))
	if err != nil {
		t.Fatalf("go.mod.tmpl 읽기 실패: %v", err)
	}
	if err := os.Remove(filepath.Join(work, "go.mod.tmpl")); err != nil {
		t.Fatalf("템플릿 제거 실패: %v", err)
	}
	gomod := strings.ReplaceAll(string(tmpl), "__SDK_ROOT__", filepath.ToSlash(filepath.Join(repoRoot, "sdk")))
	if err := os.WriteFile(filepath.Join(work, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatalf("go.mod 작성 실패: %v", err)
	}
	// go.sum 은 **SDK 모듈의 것**을 쓴다(SDK 는 독립 모듈이다).
	if sum, err := os.ReadFile(filepath.Join(repoRoot, "sdk", "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(work, "go.sum"), sum, 0o600); err != nil {
			t.Fatalf("go.sum 복사 실패: %v", err)
		}
	}

	cmd := exec.Command(goBin, "build", "./...")
	cmd.Dir = work
	cmd.Env = append(os.Environ(),
		"GOFLAGS=-mod=mod",
		"GOPROXY=off", // 네트워크 금지 — 모듈 캐시만 쓴다
		"GOWORK=off",  // 상위 go.work 가 있어도 이 모듈만 본다
		"GO111MODULE=on",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("외부 모듈에서 SDK 빌드가 실패했다(SDK 가 internal/ 을 참조하거나 자족성이 깨졌을 수 있다):\n%s", out)
	}

	// vet 까지 돌려 "빌드는 되지만 명백히 잘못된" 사용을 함께 잡는다.
	vet := exec.Command(goBin, "vet", "./...")
	vet.Dir = work
	vet.Env = cmd.Env
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("외부 모듈 vet 실패:\n%s", out)
	}
}

// repoRootDir 는 이 테스트 파일 기준으로 저장소 루트를 찾는다.
func repoRootDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("호출 위치를 알 수 없다")
	}
	// <root>/sdk/extension/v1/consumer_compile_test.go → 4단계 위가 루트
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("저장소 루트를 찾지 못했다(%s): %v", root, err)
	}
	return root
}

// copyTree 는 디렉터리를 통째로 복사한다(픽스처를 임시 디렉터리에서 빌드하기 위해).
//
// 저장소 안에서 바로 빌드하지 않는 이유: go.mod·go.sum 이 생성되면서 작업 트리가 더러워지고,
// 병렬 테스트가 서로의 파일을 덮어쓴다.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatalf("픽스처 복사 실패: %v", err)
	}
}
