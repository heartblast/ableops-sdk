# CLAUDE.md — AbleOps SDK 에이전트 지침 (Router)

> **이 파일은 매 세션 자동 로딩된다.** 여기에는 *진입점*만 둔다 — 상세는 아래 표가 가리키는 문서를
> **필요할 때만** 읽는다. 크기 예산 8KB(`scripts/verify.sh full` 이 검사한다). 상세를 여기로 되돌리지 않는다.
>
> 모든 응답·주석·커밋 메시지·테스트 이름은 **한국어**로 쓴다(기존 관례: `TestX_한글설명`).

## 1. 이 저장소는 무엇인가

`github.com/heartblast/ableops-sdk` — AbleOps Core(`ableops-kafka`)와 Extension 사이의 **공개 계약 모듈**.
Go 타입·인터페이스·Manifest 스키마·wire 프로토콜·`extserver` 런타임·`testkit` 만 담는다.
구현(Extension Registry·권한 enforcement·Kafka 클라이언트·설치/롤백)은 전부 Core 에 있다.

```text
외부 Extension(ableops-flink …) → require → ableops-sdk(이 모듈) ← require ← ableops-kafka(Core)
```

**SDK 는 Core 를 import 하지 않고, 외부 의존은 `gopkg.in/yaml.v3` 하나다.**
이 두 문장이 저장소의 존재 이유이고, 문서가 아니라 테스트(경계 게이트 12종)가 지킨다.

## 2. 기본 명령

```bash
bash scripts/verify.sh fast      # gofmt · build · vet · test -short              — 개발 루프(수 초)
bash scripts/verify.sh full      # + 전체 테스트 · 경계 게이트 12종 이름 확인 · 예제 빌드 · 의존성 최소성 — 작업 종료·PR 전 1회
bash scripts/verify.sh release   # + SDKVersion/CHANGELOG/태그 일치 · go mod tidy 무변경 · 작업트리 clean — 태그 직전 1회
go test ./extension/v1 -run TestPublicAPISnapshot                          # 공개 API 골든 비교
UPDATE_SDK_API_GOLDEN=1 go test ./extension/v1 -run TestPublicAPISnapshot  # 골든 갱신 — diff 를 눈으로 확인한다
```

CI(`.github/workflows/ci.yml`)는 `verify.sh full` 을 그대로 호출한다. **로컬 full 통과 = CI 통과.**

## 3. 작업 유형별 진입점

**먼저 이 표에서 한 줄을 고르고, 그 줄이 가리키는 문서만 읽는다.**

| 하려는 일 | 먼저 읽을 것 | 절차 |
| --- | --- | --- |
| 파일 위치를 모른다 | [파일맵.md](.claude/context/파일맵.md) | — |
| 공개 타입·필드·함수·capability·오류 코드 추가 | [불변식.md](.claude/context/불변식.md) §2 | `/add-public-api` |
| `extserver` 프로토콜·환경변수·헤더·핸드셰이크 | [불변식.md](.claude/context/불변식.md) §3 | `/add-public-api` |
| `testkit` fake·계약 점검 헬퍼 | [불변식.md](.claude/context/불변식.md) §4 | `/add-public-api` |
| `frontend/v1` 브리지 계약(TS) | [불변식.md](.claude/context/불변식.md) §5 | — |
| 예제·README·CHANGELOG 만 | [파일맵.md](.claude/context/파일맵.md) §4 | `/verify fast` |
| 어느 수준으로 검증할지 모른다 | [검증.md](.claude/context/검증.md) | `/verify` |
| 버전 올리기·태그·Core 동기화 | [릴리스.md](.claude/context/릴리스.md) | `/release` |

## 4. 절대 규칙

1. **`go.mod` 에 require 를 늘리지 않는다.** 표준 라이브러리로 해결한다. yaml.v3 외의 의존은 게이트가 거부한다.
2. **v1 공개 심볼을 제거·개명·시그니처 변경하지 않고, 인터페이스에 메서드를 추가하지 않는다.**
   필요하면 새 타입·새 인터페이스를 **추가**한다. 골든에서 줄이 **사라지면** 그것이 Breaking Change 다.
3. **문자열 값은 계약이다.** capability(`kafka.read` …)·오류 코드·환경변수 이름(`ABLEOPS_*`)·헤더 이름·
   `ABLEOPS_EXT_READY` 줄 형식은 바꾸지 않는다.
4. **`SDKVersion`(compat.go) · CHANGELOG 최신 `## vX.Y.Z` · git 태그는 함께 올린다.**
   하나만 올라간 상태는 `TestSDKVersionMatchesChangelog` 와 `verify.sh release` 가 잡는다.
5. **`full` 1회 통과 전에는 "완료"라고 보고하지 않는다.** 미실행 단계는 **"미검증"으로 명시**하고,
   실패는 **핵심 오류 라인만** 요약한다.
6. **주석은 "왜"를 쓴다.** 기존 파일의 패키지 주석·`⚠` 표기 스타일을 따른다.
   분리 이전 경로(`sdk/…`, `kafka-control-portal/…`)를 새로 쓰지 않는다(`module_gate_test.go` 의 차단 목록은 예외).
7. **`.gitattributes`(`* text=auto eol=lf`)를 지우지 않는다.** 골든은 바이트 비교라 CRLF 체크아웃에서 전부 깨진다.

## 5. Core 와의 관계 — 이 저장소에서 할 수 없는 것

- Core 가 지원하는 SDK 범위는 **Core** 가 선언한다(Core `internal/extensionhost/sdkcompat.go`).
  SDK 에 "최소 Core 버전" 상수를 두지 않는다 — 의존 화살표가 뒤집힌다.
- `frontend/v1/*.ts` 는 Core `web/src/extensions/contract/` 에 **바이트 동일** 사본이 있다.
  여기서 고치면 릴리스 뒤 Core 에 다시 복사해야 한다(Core 의 `TestFrontendContractMatchesSDK` 가 비교).
- Core 저장소의 `sdk/` 는 이 저장소의 **스테이징**이다. 그 안에서 검증할 때는 `GOWORK=off` 가 필요하다.
