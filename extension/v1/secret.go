package extensionv1

// SecretRefService — 시크릿 **참조만** 다루는 계약(capability: secret.ref).

import "context"

// SecretRef 는 시크릿을 가리키는 **참조**다. 평문 값이 아니다.
//
// Ref 는 Core 가 해석 가능한 불투명(opaque) 식별자이며(예: "cluster/prod-01/sasl-password"),
// Extension 은 이 문자열을 저장·전달·표시할 수 있지만 **값을 열 수 없다**.
// 실제 평문은 Core 의 실행 경계(예: Flink 배포 시 InjectSecrets)에서만 주입되고
// 오류·로그·감사·저장본에서는 리댁션된다.
type SecretRef struct {
	Ref string `json:"ref"`
}

// SecretRefService 는 시크릿 참조 해석 계약이다.
//
// ⚠ **어떤 메서드도 평문 시크릿을 반환하지 않는다**(요구A §19).
// Resolve 는 참조가 유효한지 확인하고 정규화된 참조를 돌려줄 뿐이며,
// Reveal/Decrypt/Value 같은 메서드를 이 인터페이스에 추가하지 않는다.
type SecretRefService interface {
	// Resolve 는 참조 문자열을 검증·정규화한다. 존재하지 않거나 접근 권한이 없으면 에러다.
	Resolve(ctx context.Context, ref string) (SecretRef, error)
}
