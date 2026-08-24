// 외부 프로세스 Extension 실행 파일 — extserver.Run 한 줄이면 된다.
package main

import (
	"fmt"
	"os"

	consumer "example.com/ableops-consumer-demo"
	"github.com/heartblast/kafka-control-portal/sdk/extension/v1/extserver"
)

func main() {
	if err := extserver.Run(consumer.New()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
