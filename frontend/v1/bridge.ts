// AbleOps Extension **Frontend Bridge v1** — 확장 화면(외부 번들)과 포털 사이의 공개 계약.
//
// ============================================================================
// 이 파일은 "계약"이고, 실행하는 쪽은 Core 다
// ============================================================================
//
// 실제 로더는 Core 프론트(web/src/extensions/remoteEntry.ts)에 있다. 이 파일에는 **타입과
// 상수만** 두어 확장 개발자가 Core 소스를 읽지 않고도 화면을 만들 수 있게 한다.
//
// 계약을 Core 안에만 두면 확장 개발자는 매번 Core 저장소를 뒤져야 하고, Core 리팩터링이 곧
// 계약 변경이 된다. 반대로 로더까지 SDK 로 옮기면 Core 의 인증·라우팅과 이중 구현이 된다.
// 그래서 **공개 계약(여기)과 로더 구현(Core)을 분리**한다.
//
// ============================================================================
// 확장 번들이 지켜야 할 것
// ============================================================================
//
// 1. 의존성 없는 **단일 ES 모듈**이어야 한다(import 로 외부 파일을 더 받지 않는다).
// 2. `mount` 를 내보낸다. 아래 셋 중 어느 형태든 된다.
//
//        export function mount(el, ctx) { … }
//        export default function mount(el, ctx) { … }
//        export default { mount(el, ctx) {…}, unmount() {…} }
//
// 3. `export const bridgeVersion = 1` 로 지원 브리지 버전을 밝힌다(생략하면 1로 본다).
// 4. **React 를 번들에 넣지 않는다.** Core 는 React 를 공유하지 않는다.
//      - 공유하면 Core 의 React 버전이 확장의 ABI 가 되어 Core 를 올릴 때마다 확장이 깨진다.
//      - 확장이 자기 React 를 담으면 한 페이지에 두 벌이 올라가 훅 상태가 깨진다.
//    프레임워크가 필요하면 확장 번들 안에서 자체적으로 쓴다(preact·lit·vanilla).
// 5. **주어진 DOM 요소 밖을 건드리지 않는다.** document.body 에 직접 붙이거나 Core 의
//    DOM·전역 상태를 조작하지 않는다. 정리 함수로 되돌릴 수 없는 변경은 만들지 않는다.
// 6. 토큰·시크릿을 다루지 않는다. 백엔드 호출은 `ctx.request` 로만 한다.
//
// ============================================================================
// ⚠ 신뢰 모델 — 이것은 보안 샌드박스가 **아니다**
// ============================================================================
//
// 확장 번들은 Core 와 **같은 브라우저 컨텍스트(same-origin)** 에서 실행되는 ES 모듈이다.
// 따라서 다음은 사실이 아니다:
//
//     "ctx 에 토큰을 넣지 않았으니 확장은 토큰을 볼 수 없다"  ← 거짓
//
// 같은 페이지에서 실행되는 JavaScript 는 `localStorage`·`document`·`fetch` 에 모두 접근할 수
// 있다. 포털의 로그인 토큰이 localStorage 에 있는 한, 확장 번들은 그것을 읽을 수 있다.
// `ctx` 가 토큰을 넘기지 않는 것은 **실수를 줄이기 위한 위생 규칙**이지 격리 수단이 아니다.
//
// 그래서 현재 정책은 이렇다:
//
//     Built-in UI(Core 번들 내장)      trusted
//     Remote ES Module(패키지 번들)     trusted package only — 신뢰하는 서명 패키지만
//     제3자·미신뢰 UI                   현재 지원하지 않는다(격리 실행 모드가 필요하다)
//
// 미신뢰 코드를 실행해야 한다면 iframe sandbox 같은 **별도 실행 모드**가 필요하며, 그것은
// 후속 과제다. 그때를 위해 Manifest 의 frontend 절은 실행 모드를 나중에 추가할 수 있도록
// 열어 둔다(FrontendKind 참조).

/**
 * Frontend Bridge 계약 버전.
 *
 * Backend 의 `ableops.io/extension/v1`(API 버전)과 **독립적으로** 올라간다.
 * 브라우저 실행 컨텍스트의 계약(mount 시그니처·ctx 필드·정리 규칙)이 바뀔 때만 상승한다.
 *
 * ⚠ 확장이 이 값보다 **높은** 버전을 요구하면 Core 는 실행하지 않고 오류를 표시한다.
 * 조용히 실행하면 없는 필드를 참조해 빈 화면이 되고, 원인은 콘솔에만 남는다.
 */
export const FRONTEND_BRIDGE_VERSION = 1;

/** Core 가 아직 받아 주는 최소 브리지 버전(구 번들 하위호환). */
export const MIN_FRONTEND_BRIDGE_VERSION = 1;

/**
 * 확장 화면의 실행 모드.
 *
 * - `trusted-module`: 현재 유일한 모드. Core 와 같은 컨텍스트에서 ES 모듈로 실행한다.
 * - `isolated-frame`: **후속 과제**. 미신뢰 제3자 UI 를 iframe 으로 격리 실행한다.
 *
 * 지금 값 하나를 위해 코드를 늘리지 않되, 이름을 미리 정의해 두어 Manifest·설치·신뢰 모델이
 * 나중에 모드를 추가할 수 있게 한다(미지정은 `trusted-module`).
 */
export type FrontendKind = 'trusted-module' | 'isolated-frame';

/** 확장 화면이 보는 현재 사용자(읽기 전용 사본). */
export interface ExtensionUser {
  /** 사용자 ID. */
  id: string;
  /** 표시 이름. */
  name: string;
  /** 포털 역할 목록. */
  roles: string[];
  /** 유효 권한 키 목록(Core 정적 권한 + ext.<id>.* 동적 권한). */
  permissions: string[];
}

/**
 * mount 가 받는 실행 컨텍스트.
 *
 * ⚠ 여기 없는 것은 계약이 아니다. Core 내부 객체(React·Router·axios·antd·store)는
 * 의도적으로 넣지 않는다 — 넣는 순간 Core 의 라이브러리 버전이 확장의 ABI 가 된다.
 */
export interface ExtensionMountContext {
  /** 브리지 계약 버전(Core 가 채운다). */
  bridgeVersion: number;
  /** 확장 ID(= Manifest 의 id). */
  extensionId: string;
  /** 설치된 확장 버전. */
  version: string;
  /** 이 확장의 프론트 경로 접두(`/extensions/<id>`). */
  basePath: string;
  /** 이 확장의 백엔드 API 접두(`/api/extensions/<id>`). */
  apiBase: string;
  /**
   * 자산 URL 을 만든다(번들이 CSS·이미지를 추가로 불러올 때).
   * 경로는 패키지의 `web/` 기준 상대 경로다.
   */
  assetUrl: (relativePath: string) => string;
  /**
   * 확장 백엔드 API 호출(`apiBase` 기준 상대 경로).
   *
   * 인증 헤더는 Core 가 붙인다 — 번들이 토큰을 보관·전송하지 않게 하기 위한 위생 규칙이다
   * (격리 수단이 아니다. 위 신뢰 모델 참조).
   */
  request: (path: string, init?: RequestInit) => Promise<Response>;
  /** 현재 사용자(로그인 정보를 알 수 없으면 null). */
  user: ExtensionUser | null;
  /** 현재 UI 언어(`ko` | `en` | `ja` | `vi`). */
  locale: string;
  /** 포털 내 이동(확장 화면에서 Core 화면으로 돌아갈 때). */
  navigate: (path: string) => void;
}

/** mount 가 돌려줄 수 있는 정리 함수. */
export type ExtensionCleanup = () => void;

/** mount 함수 시그니처. */
export type ExtensionMount = (
  el: HTMLElement,
  ctx: ExtensionMountContext,
) => void | ExtensionCleanup | Promise<void | ExtensionCleanup>;

/**
 * 확장 번들이 내보내는 모듈 형태.
 *
 * `mount` 만 필수다. `unmount` 와 `bridgeVersion` 은 선택이다.
 */
export interface ExtensionModule {
  mount: ExtensionMount;
  unmount?: () => void;
  bridgeVersion?: number;
}

/**
 * 화면 로딩 실패 사유.
 *
 * 안내 문구와 1:1 로 대응한다 — 실패를 "그냥 오류"로 뭉뚱그리면 관리자가 무엇을 고쳐야 하는지
 * 알 수 없다(entry 오타인지, 권한 문제인지, CSP 인지가 완전히 다른 조치다).
 */
export type ExtensionLoadErrorKind =
  /** 404 — 자산이 없다(entry 오타·패키지 누락). */
  | 'notFound'
  /** 403 — 이 확장 화면을 쓸 권한이 없다. */
  | 'forbidden'
  /** 503 — 확장이 비활성이거나 아직 준비되지 않았다. */
  | 'unavailable'
  /** 네트워크·서버 오류. */
  | 'network'
  /** 브라우저 CSP 가 번들 실행을 막았다. */
  | 'csp'
  /** 모듈이 mount 를 내보내지 않았다. */
  | 'contract'
  /** 번들이 요구하는 브리지 버전을 이 Core 가 지원하지 않는다. */
  | 'bridge'
  /** 번들 자체의 오류(구문 오류·실행 예외). */
  | 'runtime';

/**
 * 번들이 요구하는 브리지 버전을 이 Core 가 실행할 수 있는지 판정한다.
 *
 * 값이 없으면(구 번들) 최초 버전으로 본다 — 거부하지 않는다.
 */
export function supportsBridgeVersion(declared: unknown): boolean {
  if (declared === undefined || declared === null) return true;
  const v = Number(declared);
  if (!Number.isFinite(v) || v <= 0) return false;
  return v >= MIN_FRONTEND_BRIDGE_VERSION && v <= FRONTEND_BRIDGE_VERSION;
}
