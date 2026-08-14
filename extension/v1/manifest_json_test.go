package extensionv1

// Manifest JSON 표현 회귀 가드(적대적 검증 지적 12).
//
// Manifest 는 `GET /api/extensions` · `/{id}` · `/catalog` 응답에 **모든 확장**이 그대로 싣는다.
// 선언하지 않은 하위 구조체가 `{}` 로 새로 실리면 v1.4.0 응답과 모양이 달라져, 응답을 그대로
// 비교·스냅샷하는 소비자가 차이를 본다. encoding/json 의 omitempty 는 구조체에 적용되지 않으므로
// (Go 문서: 빈 값은 false/0/nil 포인터/빈 배열·맵·문자열뿐) 태그만 붙여 두면 조용히 어긋난다.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBackendJSON_선언하지_않은_migrations는_실리지_않는다(t *testing.T) {
	b, err := json.Marshal(BackendDecl{Enabled: true, Kind: "builtin"})
	if err != nil {
		t.Fatalf("직렬화 실패: %v", err)
	}
	if got := string(b); strings.Contains(got, "migrations") {
		t.Fatalf("선언하지 않은 migrations 가 응답에 실린다: %s", got)
	}

	// 선언한 패키지는 반드시 실려야 한다(프론트가 롤백 체크박스 판정에 쓴다).
	b, err = json.Marshal(BackendDecl{Enabled: true, Kind: "process",
		Migrations: MigrationsDecl{Rollback: MigrationRollbackDown}})
	if err != nil {
		t.Fatalf("직렬화 실패: %v", err)
	}
	if got := string(b); !strings.Contains(got, `"migrations":{"rollback":"down"}`) {
		t.Fatalf("선언한 migrations 가 응답에 없다: %s", got)
	}
}
