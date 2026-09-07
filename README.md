# AbleOps SDK

**AbleOps 플랫폼과 Extension 사이의 공개 계약 SDK.**

```go
import extensionv1 "github.com/heartblast/ableops-sdk/extension/v1"
import "github.com/heartblast/ableops-sdk/extension/v1/extserver"
```

---

## 1. 이 모듈은 무엇인가

Kafka 전용 SDK가 아니다. AbleOps Core 제품이 Extension 에게 **약속하는 것만** 담는다 —
Go 타입 · 인터페이스 · Manifest 스키마 · wire 프로토콜.

| 담는 것 | 담지 않는 것 |
| --- | --- |
| Extension · Manifest · Lifecycle | Extension Registry · 설치/삭제/업데이트 |
| Capability · Permission 선언 | 권한 enforcement · 서명 검증 · 롤백 |
| HostContext · Identity · Config · SecretRef | Host API 실제 구현 · 프로세스 감시 |
| KafkaReadService · WorkflowSubmitService · AuditService · ClusterRegistry (**인터페이스**) | Kafka 클라이언트 · DB 드라이버 · Prometheus |
| UI Menu / Route 계약 | 프론트엔드 자산 설치 |
| API/Protocol 버전 계약 · `extserver` 런타임 | Core internal 도메인 ↔ SDK DTO 어댑터 |

오른쪽 열은 전부 Core(`ableops-kafka` 의 `internal/extensionhost`)에 남는다.

### 의존 방향

```text
외부 Extension (예: ableops-flink)
        ↓ require
   ableops-sdk          ← 이 모듈
        ↑ require
 ableops-kafka (Core)
```

**SDK 는 Core 를 import 하지 않는다.** 화살표가 순환이 되는 순간 확장 개발자의 모듈 그래프에
Kafka 클라이언트 · DB 드라이버 3종 · Prometheus 가 전부 실려 나간다.
이 규칙은 문서가 아니라 테스트가 지킨다 → `extension/v1/imports_test.go` ·
`extension/v1/module_gate_test.go`.

---

## 2. 설치

```bash
go get github.com/heartblast/ableops-sdk/extension/v1
```

요구 Go 버전은 `go.mod` 의 `go` 디렉티브다(현재 **1.27.0**).
`toolchain` 줄은 이 모듈이 메인 모듈일 때만 쓰이므로 소비자 하한과 무관하다.

### 의존성

SDK 가 요구하는 외부 모듈은 **`gopkg.in/yaml.v3` 하나뿐**이다(Manifest 파싱).

다음은 **절대 들어오지 않는다** — 들어오면 게이트가 실패한다:
Kafka 클라이언트(franz-go) · DB 드라이버(pgx/mysql/go-ora) · Prometheus ·
웹 프레임워크(chi) · TUI(Bubble Tea 계열) · Core 의 어떤 패키지든.

핸들러는 표준 `net/http` 만 쓴다. 라우터를 강요하지 않기 위해서다.

---

## 3. 최소 Extension

```go
package hello

import (
	"net/http"

	extensionv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

type Extension struct{}

func (Extension) Manifest() extensionv1.Manifest {
	return extensionv1.Manifest{
		APIVersion: extensionv1.APIVersion, // "ableops.io/extension/v1"
		ID:         "hello",
		Name:       "Hello Extension",
		Version:    "1.0.0",
		Capabilities: []extensionv1.Capability{
			extensionv1.CapabilityKafkaRead,
		},
	}
}

func (Extension) Start(ctx extensionv1.StartContext) error { return nil }
func (Extension) Stop() error                              { return nil }

func (Extension) Routes() []extensionv1.Route {
	return []extensionv1.Route{{
		Method:  http.MethodGet,
		Path:    "/status",
		Handler: func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ok":true}`)) },
	}}
}
```

전체가 도는 예제는 [`examples/hello-extension`](examples/hello-extension) 에 있다.

```bash
cd examples/hello-extension && go build ./...
```

---

## 4. `extserver.Run()` — 외부 프로세스 Extension

Core 를 다시 빌드하지 않고 **독립 실행 파일**로 배포하는 형태다.
Core 가 프로세스를 띄우고, SDK 가 핸드셰이크·Host API 호출·신원 전달을 대신한다.

```go
package main

import (
	"log"

	"github.com/heartblast/ableops-sdk/extension/v1/extserver"
	"example.com/hello"
)

func main() {
	if err := extserver.Run(hello.Extension{}); err != nil {
		log.Fatal(err)
	}
}
```

`Run` 이 하는 일:

1. Core 가 넘긴 환경변수(프로토콜 버전 · Host API 주소 · 토큰 · 설정)를 읽는다.
2. 루프백에 리스너를 열고 `ABLEOPS_EXT_READY {...}` 한 줄을 stdout 으로 보낸다.
3. 호환되지 않으면 **기동하지 않고** `ABLEOPS_EXT_INCOMPATIBLE {...}` 을 보낸다.
   Core 는 이 줄을 보면 재시작하지 않고 그 확장만 격리한다(무한 재시작 방지).
4. 종료 신호를 받으면 `Stop()` 을 부르고 정리한다.

---

## 5. Capability

Extension 은 Manifest 에 선언한 capability 에 대응하는 서비스**만** 받는다.

```go
Capabilities: []extensionv1.Capability{
	extensionv1.CapabilityKafkaRead,
	extensionv1.CapabilityWorkflowSubmit,
	extensionv1.CapabilityAudit,
}
```

```go
func (e Extension) Start(ctx extensionv1.StartContext) error {
	if ctx.Host.Kafka == nil {
		// capability 미선언 = 필드가 nil 이다. 반드시 확인한다.
		return errors.New("kafka:read capability 가 필요합니다")
	}
	e.kafka = ctx.Host.Kafka
	return nil
}
```

> **선언하지 않은 capability 의 필드는 `nil` 이다** — 인터페이스는 담고 있는데 값이 없는
> typed-nil 이 아니라 진짜 nil 이다. 그래서 `== nil` 검사가 실제로 동작한다.

**Extension 은 Kafka 를 직접 변경할 수 없다.** 쓰기가 필요하면 `WorkflowSubmitService` 로
신청을 올리고 Core 의 승인·정책 검증·감사 경로를 지나야 한다. 이것은 정책이 아니라
**타입 수준의 제약**이다 — 쓰기 메서드가 SDK 에 존재하지 않는다.

capability 전체 목록은 `extensionv1.AllCapabilities()` 가 돌려준다.

---

## 6. 테스트 — `testkit`

```go
import "github.com/heartblast/ableops-sdk/extension/v1/testkit"

func TestExtension(t *testing.T) {
	host := testkit.NewHost(testkit.WithKafkaTopics("orders", "payments"))
	if err := (Extension{}).Start(testkit.StartContext(host)); err != nil {
		t.Fatal(err)
	}
	testkit.AssertManifestValid(t, Extension{}.Manifest())
}
```

`testkit` 도 공개 API 다 — 여기 있는 심볼이 바뀌면 외부 확장의 테스트가 깨진다.
그래서 공개 API 스냅샷(`extension/v1/testdata/api-v1.golden`)이 함께 고정한다.

---

## 7. 버전 정책

네 개의 버전은 **서로 독립**이다.

| 버전 | 값 | 무엇이 바뀔 때 올라가는가 |
| --- | --- | --- |
| Extension Version | Manifest `version` | 확장 자신의 기능 |
| **API Version** | `ableops.io/extension/v1` | Go 타입·인터페이스·Manifest 스키마 |
| **Protocol Version** | `1` | 환경변수·핸드셰이크·헤더·Host API 경로 |
| **SDK Version** | `extensionv1.SDKVersion` | 이 **모듈**의 릴리스(버그 수정·문서 포함) |

### SemVer

| 구분 | 기준 | 골든 파일 |
| --- | --- | --- |
| Patch | 호환 버그 수정 | 변화 없음 |
| Minor | 하위 호환 API **추가** | 줄이 추가만 됨 |
| Major | Breaking Change | v1 에서는 금지 |

### v1 이 사는 동안 금지되는 것 (Breaking Change)

- 공개 심볼·필드·메서드 **제거** 또는 시그니처 변경
- **인터페이스에 메서드 추가** ← 외부 구현체가 전부 깨진다. 새 인터페이스를 만든다
- capability · 오류 코드 · 헤더 · 환경변수의 **문자열 값** 변경
- 기존 capability 의 **의미 확대**(read 에 쓰기를 얹는 등)
- Manifest 에 **필수** 필드 추가

Breaking Change 가 필요하면 `extension/v2` 를 **새로 만든다.** v1 은 그대로 둔다.

자동 감시:

```bash
go test ./extension/v1 -run TestPublicAPISnapshot   # 공개 심볼 골든
go test ./extension/v1 -run TestExternalConsumer    # 외부 모듈에서 실제 컴파일
```

골든을 갱신할 때는 `UPDATE_SDK_API_GOLDEN=1` 을 쓰고 **diff 를 눈으로 확인한다.**
줄이 사라졌으면 그것은 파괴적 변경이다.

### Core 와의 호환성

| AbleOps SDK | AbleOps Core |
| --- | --- |
| v1.0.x | v1.7.0 이상 |

- Core 는 자신이 지원하는 SDK 범위를 스스로 선언한다
  (Core 저장소 `internal/extensionhost/sdkcompat.go`).
- 개별 확장은 Manifest 의 `requires.core` 로 더 좁은 제약을 걸 수 있다:

  ```yaml
  requires:
    core: ">=1.7.0 <2.0.0"   # 비우면 제한 없음
  ```

- Core 버전이 `dev`(개발 빌드)이면 `requires.core` 검사를 생략한다.
  `apiVersion` 불일치는 개발 빌드에서도 **거부**한다 — 계약 자체가 다르기 때문이다.

---

## 8. 기존 확장 마이그레이션

SDK 저장소 분리로 바뀌는 것은 **import 경로 하나뿐**이다.
Go 타입·인터페이스·Manifest 스키마·wire 프로토콜은 그대로다.

```bash
# 1) import 경로 치환
grep -rl 'github.com/heartblast/kafka-control-portal/sdk' . \
  | xargs sed -i 's|github.com/heartblast/kafka-control-portal/sdk|github.com/heartblast/ableops-sdk|g'

# 2) 모듈 요구 교체
go mod edit -droprequire github.com/heartblast/kafka-control-portal/sdk
go get github.com/heartblast/ableops-sdk@v1.0.0
go mod tidy

# 3) 확인
go build ./... && go test ./...
```

이미 배포된 확장 **패키지·바이너리는 다시 만들지 않아도 된다.**
apiVersion 과 프로토콜 버전이 그대로이므로 Core 는 기존 패키지를 그대로 로드한다.

---

## 9. 저장소와 기여

- Core 제품: `github.com/heartblast/ableops-kafka`
- 이 SDK: `github.com/heartblast/ableops-sdk`
- 참조 외부 Extension: `github.com/heartblast/ableops-flink`

`go test ./...` 는 **네트워크 없이** 통과해야 한다. 외부 consumer 검증은 모듈 캐시만 쓴다.
PR 을 올리기 전에:

```bash
go build ./... && go vet ./... && go test ./...
```
