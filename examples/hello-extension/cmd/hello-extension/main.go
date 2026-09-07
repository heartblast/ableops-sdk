// hello-extension 은 예제 확장을 **외부 프로세스**로 실행하는 진입점이다.
//
// Core 는 이 실행파일을 띄우고 stdout 한 줄(ABLEOPS_EXT_READY)로 핸드셰이크를 받는다.
// 그 절차 전부 — 환경변수 해석 · 리스너 · 토큰 · 신원 전달 · 종료 처리 — 를 extserver.Run 이 한다.
package main

import (
	"log"

	hello "github.com/heartblast/ableops-sdk/examples/hello-extension"
	"github.com/heartblast/ableops-sdk/extension/v1/extserver"
)

func main() {
	if err := extserver.Run(hello.New()); err != nil {
		log.Fatalf("확장 실행 실패: %v", err)
	}
}
