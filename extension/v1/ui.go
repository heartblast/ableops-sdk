package extensionv1

// Extension 의 프론트엔드 기여(contribution) 선언 — 라우트와 메뉴.
//
// Manifest 는 **데이터만** 선언한다. JS/JSX·컴포넌트·스타일을 Manifest 로 전달하지 않으며
// Core 프론트가 이를 실행하는 일도 없다(임의 코드 실행 방지).

// RouteDecl 은 Extension 이 제공하는 백엔드 REST 라우트 1건의 선언이다.
//
// Path 는 반드시 `/api/extensions/<extensionID>` 접두여야 한다(Manifest 검증이 강제).
// Permission 은 이 경로 접근에 필요한 권한 키이며, 비우면 인증만 요구한다.
// `ext.` 로 시작하면 반드시 자기 Extension 네임스페이스여야 한다.
type RouteDecl struct {
	Path       string `json:"path" yaml:"path"`
	Permission string `json:"permission,omitempty" yaml:"permission,omitempty"`
}

// MenuDecl 은 Extension 이 좌측 내비게이션에 추가할 메뉴 1건의 선언이다.
//
// Parent 는 상위 메뉴 키다. 비우면 최상위 그룹이고, Core 그룹 키(예: ops-env)를 지정하면 그 아래 항목이 된다.
// 메뉴는 **2단(그룹 > 항목)까지만** 허용한다 — Core 프론트의 권한 필터(visibleMenu)가 손자 항목을
// 조용히 버리기 때문에, 3단 선언은 "메뉴가 있는데 안 보인다"로만 드러난다. Manifest 검증에서 거부한다.
//
// Icon 은 **문자열 ID** 이며(예: "appstore"), Core 프론트의 whitelist 를 통해서만 실제 아이콘 컴포넌트로
// 변환된다. whitelist 에 없는 ID 는 기본 아이콘으로 대체되고, 임의 컴포넌트/코드로 해석되지 않는다.
type MenuDecl struct {
	Key        string `json:"key" yaml:"key"`
	Parent     string `json:"parent,omitempty" yaml:"parent,omitempty"`
	Path       string `json:"path" yaml:"path"` // `/extensions/<extensionID>` 접두
	Label      string `json:"label" yaml:"label"`
	Icon       string `json:"icon" yaml:"icon"`
	Permission string `json:"permission,omitempty" yaml:"permission,omitempty"`
}
