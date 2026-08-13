// Package extserver 는 **외부 프로세스 Extension**(Manifest 의 `backend.kind: process`)을
// 한 함수로 구현하게 해 주는 SDK 런타임이다.
//
//	func main() {
//	    if err := extserver.Run(myext.New()); err != nil {
//	        fmt.Fprintln(os.Stderr, err)
//	        os.Exit(1)
//	    }
//	}
//
// Run 은 HTTP 서버 기동·기동 핸드셰이크·호출 토큰 검증·Identity 헤더 복원·Host API 클라이언트
// 조립·graceful shutdown 을 전부 담당한다. Extension 작성자는 extensionv1.Extension 계약
// (Manifest/Start/Stop/Health/Routes)만 구현하면 된다.
//
// # 왜 필요한가
//
// 이 계층이 없으면 외부 프로세스 Extension 을 만들 때마다 개발자가 HTTP 서버·핸드셰이크 규약·
// 헤더 파싱·토큰 검증·Host API 클라이언트를 직접 짜야 한다. 그중 하나라도 틀리면 증상은
// "왜인지 확장이 안 뜬다"로만 드러나고, 토큰 비교를 == 로 짜는 것 같은 보안 실수도 각자 반복된다.
// 계약 구현을 SDK 가 한 곳에서 책임지면 그런 실수가 구조적으로 사라진다.
//
// # 프로토콜 요약(외부 프로세스 Extension 프로토콜 v1)
//
// Core → Extension 기동(환경변수, env.go):
//
//	ABLEOPS_EXT_ID · ABLEOPS_EXT_VERSION · ABLEOPS_EXT_CALL_TOKEN ·
//	ABLEOPS_HOST_URL · ABLEOPS_HOST_TOKEN · ABLEOPS_EXT_CAPABILITIES · ABLEOPS_EXT_CONFIG
//
// ⚠ 평문 시크릿(Kafka/DB/ES 비밀번호·마스터키)은 **어떤 환경변수로도 전달되지 않는다**.
// 위 두 토큰은 Core↔Extension 호출 인증용이며 로그·오류·핸드셰이크 어디에도 출력하지 않는다.
//
// Extension → Core 기동 핸드셰이크(stdout 한 줄, run.go):
//
//	ABLEOPS_EXT_READY {"addr":"127.0.0.1:54321","version":"1.0.0"}
//
// 포트는 Core 가 미리 고르지 않는다 — 확보와 사용 사이에 다른 프로세스가 그 포트를 채가는 race 가
// 있기 때문이다. Extension 이 127.0.0.1:0 으로 잡고 **실제 주소**를 알려준다.
// 주소는 항상 루프백이다(외부 노출 차단). 이 줄 뒤의 stdout/stderr 는 Core 가 로그로 수집한다.
//
// Core → Extension 요청 프록시(identity.go):
//
//	/api/extensions/{id}/<나머지>  →  http://<addr>/<나머지>
//	X-Ableops-Call-Token · X-Ableops-Extension-Id · X-Ableops-Request-Id ·
//	X-Ableops-User-Id · X-Ableops-User-Name · X-Ableops-User-Department ·
//	X-Ableops-Roles · X-Ableops-Permissions
//
// Core 는 클라이언트가 보낸 X-Ableops-* 헤더를 **전부 삭제한 뒤 자신이 다시 만든다**.
// 따라서 이 헤더는 신뢰할 수 있으며, extserver 는 그 값을 extensionv1.Identity 로 복원해
// 요청 컨텍스트에 넣는다(IdentityFrom 으로 꺼낸다).
//
// # 헤더 값 인코딩 — URL 인코딩(application/x-www-form-urlencoded)
//
// HTTP 헤더 값은 ISO-8859-1 범위를 전제로 하므로 사용자 이름·부서에 한글이 들어가면 그대로 실을 수 없다
// (Go 의 net/http 는 비-ASCII 헤더 값을 그대로 통과시키지만, 프록시·게이트웨이를 지나며 깨지거나
// 요청이 거부될 수 있다). 그래서 **신원 헤더 값은 URL 인코딩해 전달한다**:
//
//	인코딩: net/url.QueryEscape  (공백 → '+', 그 외 비예약 문자 → %XX)
//	디코딩: net/url.QueryUnescape (EncodeHeaderValue / DecodeHeaderValue)
//
// Core 와 Extension 이 **같은 방식**을 써야 하므로, Core 측 프록시도 EncodeHeaderValue 와 동일한
// 인코딩(QueryEscape)을 사용해야 한다. 디코딩은 관대하게 동작한다 — 인코딩되지 않은 순수 ASCII 값도
// 그대로 통과한다(이전 버전 Core 와의 하위호환).
//
// ⚠ 예외는 역방향 호출의 X-Ableops-On-Behalf-Of 하나다. Core 가 그 값을 단기 요청 토큰에 서명해
// 넣은 원본 사용자 ID 와 **그대로 비교**하므로 인코딩하지 않고 원문을 보낸다(identity.go 주석 참조).
//
// # capability 최소권한
//
// HostContext 의 각 서비스는 Host API 를 호출하는 HTTP 클라이언트 구현이며(hostclient.go),
// **Manifest 가 선언하지 않은 capability 필드는 nil 로 남긴다**. in-process Extension 과 똑같이
// "선언하지 않은 기능은 권한 오류가 아니라 사용 자체가 불가능"이 프로세스 경계에서도 성립한다.
// nil 포인터를 인터페이스에 담으면 인터페이스 자체는 non-nil 이 되어 최소권한이 무력화되므로
// (typed-nil 함정) 조건부 대입만 한다.
//
// # 파일 지도
//
//	doc.go        이 문서(프로토콜·인코딩 규약)
//	env.go        환경변수 계약(Environment 로딩·검증·마스킹)
//	identity.go   X-Ableops-* 헤더 ↔ extensionv1.Identity 변환과 요청 컨텍스트
//	hostclient.go Host API 클라이언트(= HostContext 의 각 서비스 구현)
//	logger.go     stderr 구조화 로거(핸드셰이크 줄을 오염시키지 않도록 stdout 을 쓰지 않는다)
//	run.go        진입점·수명주기(기동 → 핸드셰이크 → 서빙 → graceful shutdown)
//
// # 의존성
//
// 이 패키지는 표준 라이브러리와 상위 SDK 패키지(extensionv1)만 사용한다.
// internal/ · chi · 외부 모듈을 import 하지 않는다(sdk/extension/v1/imports_test.go 가 강제).
package extserver
