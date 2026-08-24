// Package testkit 은 **외부 Extension 개발자를 위한 공개 테스트 도구**다.
//
// Extension 을 테스트하려면 HostContext 가 필요한데, 그것을 손으로 만들려면 capability 서비스
// 인터페이스를 전부 구현해야 한다. 그 부담 때문에 "테스트는 Core 를 띄워서 한다"가 되면
// Extension 개발이 Core 저장소에 묶인다 — SDK 를 공개하는 의미가 사라진다.
//
//	host := testkit.NewHost("my-ext",
//	    testkit.WithKafka(testkit.NewFakeKafka().WithTopics("prod-01", extensionv1.TopicInfo{Name: "orders"})),
//	    testkit.WithIdentity(extensionv1.Identity{UserID: "owner01", Roles: []string{"ServiceOwner"}}),
//	)
//	if err := ext.Start(context.Background(), host); err != nil { … }
//
// # 철칙
//
//  1. **internal/ 을 import 하지 않는다.** Core 의 fake 를 복사해 노출하지 않고 SDK 타입으로
//     새로 정의한다. 어기면 외부 모듈에서 컴파일조차 되지 않는다(Go internal 규칙).
//  2. **capability 최소권한을 그대로 재현한다.** 옵션으로 주지 않은 capability 는 nil 이며,
//     확장은 실제 Core 에서와 똑같이 CAPABILITY_UNAVAILABLE 을 받는다. testkit 이 관대하면
//     "테스트는 통과하는데 설치하면 안 되는" 확장이 만들어진다.
//  3. **표준 라이브러리 + SDK 만 의존한다.** testing 패키지에도 의존하지 않는다 — 확장의
//     통합 테스트·로컬 실행 도구에서도 그대로 쓸 수 있어야 한다.
//
// # 파일 지도
//
//	doc.go       이 문서
//	host.go      NewHost 와 옵션(HostContext 조립)
//	fakes.go     capability 서비스 6종의 가짜 구현
//	contract.go  Manifest·라우트·생애주기·Health 계약 점검 헬퍼
package testkit
