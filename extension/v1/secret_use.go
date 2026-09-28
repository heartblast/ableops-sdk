package extensionv1

// secret.use — Host 가 수행하는 인증 egress(외부 API 호출에 시크릿을 쓰는 경로).
//
// SecretRefService 의 원칙은 그대로다: **평문 시크릿은 Extension 에 돌아오지 않는다.**
// LLM API 처럼 시크릿으로 인증해야 하는 외부 호출은 요청을 Host 에 맡긴다.
//
//	Extension ── SecretRef + HTTP 요청 ──▶ Host
//	                                        ├ ref 가 **이 Extension 에 바인딩**되었는지 확인
//	                                        ├ 대상 origin 이 바인딩의 허용 목록에 있는지 확인
//	                                        ├ 시크릿을 Host 의 Secret Store 에서 해석
//	                                        ├ 바인딩이 정한 헤더에만 자격증명을 넣어 호출(리다이렉트 추적 없음)
//	                                        └ 매 사용을 감사(ref·origin·결과 — 값 없음)
//	Extension ◀── 상류 응답(스트리밍) ──────┘
//
// 평문은 Extension 프로세스 메모리·로그·오류·감사 어디에도 들어오지 않는다. 자격증명을 넣을
// 헤더 이름과 허용 origin 은 관리자가 Host 에서 정한다 — Extension 이 고를 수 없다(키를 임의
// 목적지로 보내게 만들 수 없다). 쿼리 파라미터 주입은 지원하지 않는다(URL 은 로그·오류에 실린다).

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// CapSecretUse 는 SecretUseService(Host 수행 인증 egress)를 허용한다.
// 허용: Host 에 바인딩된 시크릿 참조로 허용된 origin 에 HTTP 요청 위임.
// 불허: 평문 시크릿 획득. 어떤 메서드도 평문을 반환하지 않는다.
const CapSecretUse Capability = "secret.use"

// MaxSecretEgressRequestBytes 는 위임 요청 본문 상한이다(Host 도 같은 상한을 강제한다).
const MaxSecretEgressRequestBytes = 8 << 20

// SecretEgressRequest 는 Host API `POST /secrets/egress` 의 요청 본문이다.
//
// 응답은 상류 응답을 그대로 스트리밍한다. Host 는 상류가 준 응답에 HeaderEgressUpstream
// (extserver)을 붙이며, 그 헤더가 없는 오류 응답은 Host 자신의 거부다(SDK Error Contract).
type SecretEgressRequest struct {
	Ref    string      `json:"ref"`
	Method string      `json:"method"`
	URL    string      `json:"url"`
	Header http.Header `json:"header,omitempty"`
	Body   []byte      `json:"body,omitempty"`
}

// SecretUseService 는 시크릿으로 인증한 외부 호출을 Host 에 위임하는 계약이다(capability: secret.use).
//
// ⚠ **평문을 돌려주는 메서드는 없다**. Reveal/Value 같은 메서드를 추가하지 않는다.
//
// Do 는 req 를 Host 를 통해 보낸다. req 에 넣은 자격증명 헤더(Authorization 등)는 Host 가
// 버린다. 상류가 응답했다면(상태코드와 무관하게) *http.Response 를 돌려주며 Body 는 호출자가
// 닫는다. Host 거부는 코드가 붙은 오류다:
//
//	NOT_FOUND          ref 가 없거나 이 Extension 에 바인딩되지 않음(존재 여부를 구분하지 않는다)
//	PERMISSION_DENIED  origin·메서드가 바인딩에서 허용되지 않음
//	INVALID_ARGUMENT   URL·헤더·본문이 계약을 벗어남
//	HOST_UNAVAILABLE   Secret Store 를 지금 사용할 수 없음
//	TEMPORARY_FAILURE  상류에 연결하지 못함
//
// 컨텍스트 취소·기한은 상류 호출까지 전파된다.
type SecretUseService interface {
	Do(ctx context.Context, ref SecretRef, req *http.Request) (*http.Response, error)
}

// RequireSecretUse 는 secret.use 서비스를 돌려준다(없으면 CAPABILITY_UNAVAILABLE).
func (h HostContext) RequireSecretUse() (SecretUseService, error) {
	return RequireService[SecretUseService](h, CapSecretUse)
}

// SecretTransport 는 요청을 svc 로 위임하는 http.RoundTripper 다.
//
// 표준 http.Client(또는 그 위의 LLM SDK)에 끼워 쓰면 Extension 코드는 평소처럼 요청을 만들고,
// 자격증명은 Host 가 넣는다:
//
//	svc, err := host.RequireSecretUse()
//	client := &http.Client{Transport: extensionv1.SecretTransport(svc, extensionv1.SecretRef{Ref: "llm/api-key"})}
func SecretTransport(svc SecretUseService, ref SecretRef) http.RoundTripper {
	return secretTransport{svc: svc, ref: ref}
}

type secretTransport struct {
	svc SecretUseService
	ref SecretRef
}

func (t secretTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.svc == nil || isNilService(t.svc) {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, NewError(CodeCapabilityUnavailable, "secret.use 서비스가 없습니다(Manifest capabilities 와 Host 허용을 확인하세요)")
	}
	return t.svc.Do(req.Context(), t.ref, req)
}

// SecretEgressOrigin 은 URL 의 origin(scheme://host[:port], 소문자, 기본 포트 생략)을 돌려준다.
//
// Host 와 테스트 Host 가 **같은 규칙**으로 허용 origin 을 비교하게 하려고 공개한다.
// http·https 가 아니거나, host 가 없거나, userinfo·fragment 가 있으면 오류다.
func SecretEgressOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", NewError(CodeInvalidArgument, "egress URL 을 해석할 수 없습니다")
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "https" && scheme != "http") || u.Host == "" || u.Opaque != "" {
		return "", NewError(CodeInvalidArgument, "egress URL 은 http(s) 절대 URL 이어야 합니다")
	}
	if u.User != nil || u.Fragment != "" {
		return "", NewError(CodeInvalidArgument, "egress URL 에 userinfo·fragment 를 넣을 수 없습니다")
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		return scheme + "://" + host + ":" + port, nil
	}
	return scheme + "://" + host, nil
}

// SanitizeSecretEgressHeader 는 위임 요청 헤더에서 Extension 이 보내면 안 되는 헤더를 뺀 사본을 만든다.
//
// 빠지는 것: hop-by-hop·Host·Content-Length, 모든 자격증명 헤더(Authorization·Proxy-Authorization·
// Cookie), X-Ableops-* (Host 네임스페이스), 그리고 inject(바인딩이 자격증명을 넣을 헤더).
// Host 는 이 사본에 자격증명을 넣는다 — Extension 이 같은 헤더를 미리 채워 덮어쓸 수 없다.
func SanitizeSecretEgressHeader(in http.Header, inject string) http.Header {
	out := http.Header{}
	drop := map[string]bool{
		"Authorization": true, "Proxy-Authorization": true, "Cookie": true, "Host": true,
		"Content-Length": true, "Connection": true, "Keep-Alive": true, "Proxy-Connection": true,
		"Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
	}
	if inject != "" {
		drop[http.CanonicalHeaderKey(inject)] = true
	}
	for k, v := range in {
		ck := http.CanonicalHeaderKey(k)
		if drop[ck] || strings.HasPrefix(ck, "X-Ableops-") {
			continue
		}
		out[ck] = append([]string(nil), v...)
	}
	return out
}
