---
description: SDK 공개 API(타입·필드·함수·capability·오류 코드·extserver 프로토콜 요소·testkit 헬퍼)를 추가·변경할 때의 절차. v1 하위호환 판정 → 구현 4곳 동기화 → 골든 갱신·diff 검토 → CHANGELOG 까지.
---

# add-public-api — 공개 API 추가

## 목적

이 저장소의 거의 모든 코드 변경은 **외부 확장 개발자의 컴파일 결과를 바꾼다.**
그래서 "구현이 되는가"보다 "**하위호환인가**"를 먼저 판정하고, 계약이 사는 4곳(타입 · Host 주입 · testkit fake · extserver 원격 구현)을 함께 고친다.

## 실행 절차

1. **호환 판정을 먼저 한다** — [불변식.md](../context/불변식.md) §2.
   - 심볼 제거·개명·시그니처 변경, 인터페이스 메서드 추가, 문자열 값 변경, Manifest 필수 필드 추가 → **v1 에서 불가.**
     사용자에게 "Breaking 이므로 v2 신설 또는 대안(새 인터페이스 + `RequireX` 헬퍼)" 을 보고하고 멈춘다.
   - 추가만 한다면 진행(Minor). 추가 없는 수정이면 Patch.
2. **어디를 고치는지 [파일맵.md](../context/파일맵.md) §2·§3 에서 찾는다.** 특히:
   - **새 capability**: `capability.go`(상수·`AllCapabilities`) → `host.go`(필드·`RequireX`) → `services.go`(조회) →
     `testkit/fakes.go` + `testkit/host.go`(fake·`WithX`) → `extserver/hostclient.go`(원격 구현) → `extserver/env.go`(협상).
     하나라도 빠지면 in-process 는 되고 외부 프로세스는 안 되는(또는 반대) 확장이 생긴다.
   - **새 서비스 메서드가 필요하면 기존 인터페이스에 넣지 않는다.** 새 인터페이스를 만들고 `WithService` 경로로 주입한다.
   - **새 오류 코드**: `errors.go` 상수 + HTTP 매핑 + `RetryableCode` 판정 + `errors_test.go` 의 상수값 테스트.
   - **새 환경변수·헤더**: 선택(optional)으로만. `env.go` 의 누락 안내 목록 · `Environment.String` 마스킹 대상 여부 확인.
3. **주석은 "왜"를 쓴다.** 필드에는 nil/빈값의 의미, 문자열 상수에는 "값은 계약이다" 표기. 기존 파일 스타일을 따른다.
4. **테스트를 붙인다.** 이름은 `TestX_한글설명`. 문자열 값 상수는 반드시 값 고정 테스트(`TestCapability_상수값` 참조).
5. **골든을 갱신하고 diff 를 읽는다.**
   ```bash
   UPDATE_SDK_API_GOLDEN=1 go test ./extension/v1 -run TestPublicAPISnapshot
   git diff extension/v1/testdata/api-v1.golden
   ```
   `-` 줄이 하나라도 있으면 Breaking 이다 — 1 로 돌아간다. `+  imethod …` 도 Breaking 이다.
6. **README 에 소비자용 예제가 필요하면 `examples/hello-extension` 을 먼저 고치고 README 를 맞춘다.**
   새 코드 블록은 실제로 컴파일되는지 확인한다(README 예제가 API 와 어긋났던 전례가 있다).
7. **CHANGELOG 에 적는다.** 릴리스 중이 아니면 `## 미출시` 절(`## v` 로 시작하지 않아야 게이트가 무시한다)에 항목을 쌓는다.
8. `/verify full`.

## 최종 보고 형식

- 호환 판정 결과(Patch/Minor)와 근거(골든 diff 요약: 추가 N줄, 삭제 0줄)
- 고친 파일 목록(4곳 동기화 여부 명시)
- `verify.sh full` 결과 표. 미실행 단계는 "미검증"
