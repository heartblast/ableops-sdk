// AbleOps Extension Frontend SDK v1 진입점.
//
// 확장 개발자는 이 파일 하나만 보면 된다(타입·상수 전부 여기서 다시 내보낸다).
// 구현은 없다 — 실행은 Core 로더가 한다(bridge.ts 상단 주석 참조).

export {
  FRONTEND_BRIDGE_VERSION,
  MIN_FRONTEND_BRIDGE_VERSION,
  supportsBridgeVersion,
} from './bridge';

export type {
  ExtensionCleanup,
  ExtensionLoadErrorKind,
  ExtensionModule,
  ExtensionMount,
  ExtensionMountContext,
  ExtensionUser,
  FrontendKind,
} from './bridge';
