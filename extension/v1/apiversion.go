package extensionv1

// Extension API 버전 식별자와 호환 판정.

import "strings"

// APIVersion 은 이 SDK 가 정의하는 Extension API 버전이다.
// Manifest 의 apiVersion 필드는 이 값과 **정확히** 일치해야 한다.
const APIVersion = "ableops.io/extension/v1"

// SupportsAPIVersion 은 주어진 apiVersion 문자열을 이 SDK 가 처리할 수 있는지 판정한다.
//
// 앞뒤 공백만 허용(트림)하고 그 외에는 **정확히 일치**할 때만 true 다.
// 빈 값은 false — apiVersion 미기재를 "기본값 v1" 으로 봐주면, 스키마가 다른 미래 버전 패키지가
// 조용히 v1 으로 해석되어 필드 유실이 감지되지 않는다.
func SupportsAPIVersion(v string) bool {
	return strings.TrimSpace(v) == APIVersion
}
