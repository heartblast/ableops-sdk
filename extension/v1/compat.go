package extensionv1

// SDK 모듈 자신의 릴리스 식별 — **Core 버전은 여기서 말하지 않는다.**
//
// # 왜 SDK 는 Core 버전을 모르는가
//
// 의존 방향은 한 방향이다(요청 §2):
//
//	외부 Extension → ableops-sdk ← ableops-kafka(Core)
//
// SDK 가 "최소 Core 버전"을 상수로 들면 그 화살표가 뒤집힌다. Core 가 1.8 을 내는 순간
// SDK 를 고쳐야 하고, 그러면 「SDK 버전은 SDK 공개 API 변경을 기준으로만 올린다」(요청 §6)가
// 성립하지 않는다. **어떤 SDK 계열을 지원하는지는 Core 가 선언한다**
// (Core: internal/extensionhost/sdkcompat.go).
//
// # 네 개의 버전축과 이 상수의 자리
//
//	Extension Version   확장 자신의 버전(Manifest version)
//	API Version         Go 타입·Manifest 스키마 계약 → APIVersion(apiversion.go)
//	Protocol Version    프로세스 wire 계약           → ProtocolVersion(protocol.go)
//	SDK Version         이 **모듈**의 릴리스 버전     → SDKVersion(아래)
//
// SDKVersion 은 계약이 아니라 **배포 단위**의 버전이다. API 가 그대로여도(버그 수정·문서·
// 테스트킷 개선) 올라가고, 반대로 SDKVersion 이 올라가도 APIVersion 은 그대로일 수 있다.

// SDKVersion 은 이 SDK 모듈의 릴리스 버전이다(모듈 태그 `vX.Y.Z` 의 숫자 부분).
//
// ⚠ 태그·CHANGELOG 와 **함께** 올린다. 셋 중 하나만 올라간 상태는 게이트가 잡는다
// (compat_test.go 의 TestSDKVersionMatchesChangelog).
//
// SemVer 규칙(요청 §6):
//
//	Patch  호환 버그 수정 — 공개 심볼 변화 없음
//	Minor  하위 호환 API 추가 — 골든(testdata/api-v1.golden)에 줄이 **추가만** 된다
//	Major  Breaking Change — v1 에서는 금지다. 새 API 버전(extension/v2)을 만든다
const SDKVersion = "1.0.0"
