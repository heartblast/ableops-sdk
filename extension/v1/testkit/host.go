package testkit

// HostContext 조립 — 확장 테스트에서 Core 없이 실행 컨텍스트를 만든다.

import (
	"context"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
)

// Host 는 조립된 테스트용 실행 컨텍스트다.
//
// HostContext 자체가 아니라 감싼 타입을 돌려주는 이유: 확장에 넘길 HostContext 와, 테스트가
// 검증에 쓸 가짜 구현들을 함께 들고 있어야 하기 때문이다. Context() 로 HostContext 를 꺼낸다.
type Host struct {
	ctx extv1.HostContext

	// Kafka·Clusters·Workflow·Audit·Config·Secrets 는 주입한 가짜 구현이다(미주입이면 nil).
	// 확장이 무엇을 요청했는지 검증할 때 여기서 이력을 읽는다.
	Kafka    *FakeKafka
	Clusters *FakeClusters
	Workflow *FakeWorkflow
	Audit    *FakeAudit
	Config   *FakeConfig
	Secrets  *FakeSecrets
	// Log 는 기록형 로거다(항상 주입된다 — 시크릿 유출 검증에 쓴다).
	Log *RecordingLogger

	// extra 는 WithService 로 추가된 임의 capability 서비스다.
	extra []serviceEntry
}

// Context 는 확장의 Start 에 넘길 HostContext 를 돌려준다.
func (h *Host) Context() extv1.HostContext { return h.ctx }

// Option 은 Host 조립 옵션이다.
type Option func(*Host)

// NewHost 는 테스트용 HostContext 를 조립한다.
//
//	host := testkit.NewHost("my-ext", testkit.WithKafka(testkit.NewFakeKafka()))
//	err := ext.Start(ctx, host.Context())
//
// ⚠ **옵션으로 주지 않은 capability 는 nil 이다.** 실제 Core 와 같은 최소권한을 재현한다 —
// 관대하게 전부 채워 주면 "테스트는 통과하는데 Manifest 에 선언하지 않아 설치하면 죽는" 확장이
// 만들어진다.
func NewHost(extensionID string, opts ...Option) *Host {
	h := &Host{
		ctx: extv1.HostContext{ExtensionID: extensionID},
		Log: NewRecordingLogger(),
	}
	for _, apply := range opts {
		if apply != nil {
			apply(h)
		}
	}
	h.ctx.Log = h.Log
	h.ctx.Services = h.buildResolver()
	return h
}

// buildResolver 는 주입된 것만 담은 Resolver 를 만든다(typed-nil 금지 — SDK services.go).
func (h *Host) buildResolver() extv1.ServiceResolver {
	services := extv1.ServiceMap{}
	if h.Kafka != nil {
		services[extv1.CapKafkaRead] = extv1.KafkaReadService(h.Kafka)
	}
	if h.Clusters != nil {
		services[extv1.CapClusterRead] = extv1.ClusterRegistry(h.Clusters)
	}
	if h.Workflow != nil {
		services[extv1.CapWorkflowSubmit] = extv1.WorkflowSubmitService(h.Workflow)
	}
	if h.Audit != nil {
		services[extv1.CapAuditWrite] = extv1.AuditService(h.Audit)
	}
	if h.Secrets != nil {
		services[extv1.CapSecretRef] = extv1.SecretRefService(h.Secrets)
	}
	if h.Config != nil {
		services[extv1.CapConfigRead] = extv1.ConfigReadService(h.Config)
		if h.Config.writable {
			services[extv1.CapConfigWrite] = extv1.ConfigWriteService(h.Config)
		}
	}
	// 마지막에 적용해 명시적 주입이 기본 조립을 이기게 한다.
	for _, e := range h.extra {
		services[e.capability] = e.svc
	}
	return services
}

// WithKafka 는 kafka.read 를 주입한다.
func WithKafka(f *FakeKafka) Option {
	return func(h *Host) {
		if f == nil {
			return
		}
		h.Kafka = f
		h.ctx.Kafka = f
	}
}

// WithClusters 는 cluster.read 를 주입한다.
func WithClusters(f *FakeClusters) Option {
	return func(h *Host) {
		if f == nil {
			return
		}
		h.Clusters = f
		h.ctx.Clusters = f
	}
}

// WithWorkflow 는 workflow.submit 을 주입한다.
func WithWorkflow(f *FakeWorkflow) Option {
	return func(h *Host) {
		if f == nil {
			return
		}
		h.Workflow = f
		h.ctx.Workflow = f
	}
}

// WithAudit 는 audit.write 를 주입한다.
func WithAudit(f *FakeAudit) Option {
	return func(h *Host) {
		if f == nil {
			return
		}
		h.Audit = f
		h.ctx.Audit = f
	}
}

// WithConfig 는 config.read 를 주입한다(**조회 전용** — Set 은 PERMISSION_DENIED).
func WithConfig(values map[string]any) Option {
	return func(h *Host) {
		cfg := NewFakeConfig(values)
		h.Config = cfg
		h.ctx.Config = cfg
	}
}

// WithWritableConfig 는 config.read + config.write 를 함께 주입한다.
//
// 조회 전용과 나누는 이유: 확장이 Set 을 부르는데 Manifest 에 config.write 를 넣지 않았다면
// 실제 설치에서 막힌다. 테스트에서도 그 차이가 드러나야 한다.
func WithWritableConfig(values map[string]any) Option {
	return func(h *Host) {
		cfg := NewFakeConfig(values)
		cfg.writable = true
		h.Config = cfg
		h.ctx.Config = cfg
	}
}

// WithSecrets 는 secret.ref 를 주입한다(존재한다고 볼 참조 이름 목록).
func WithSecrets(known ...string) Option {
	return func(h *Host) {
		f := NewFakeSecrets(known...)
		h.Secrets = f
		h.ctx.Secrets = f
	}
}

// WithIdentity 는 모든 요청에서 같은 사용자를 돌려주는 신원 해소기를 주입한다.
//
// Identity 는 capability 가 아니라 실행 컨텍스트다 — Core 도 선언 여부와 무관하게 주입한다.
func WithIdentity(id extv1.Identity) Option {
	return func(h *Host) {
		h.ctx.Identity = func(context.Context) (extv1.Identity, bool) { return id, true }
	}
}

// WithIdentityFunc 는 요청마다 달라지는 신원 해소기를 주입한다(익명 요청 재현 등).
func WithIdentityFunc(fn func(context.Context) (extv1.Identity, bool)) Option {
	return func(h *Host) {
		if fn != nil {
			h.ctx.Identity = fn
		}
	}
}

// WithLogger 는 기록형 로거를 교체한다(여러 Host 가 같은 로거를 공유할 때).
func WithLogger(l *RecordingLogger) Option {
	return func(h *Host) {
		if l != nil {
			h.Log = l
		}
	}
}

// WithService 는 **임의 capability** 의 서비스를 Resolver 로 주입한다.
//
// v1 이 필드를 갖지 않는 신규 capability(config.write 등)를 테스트할 때 쓴다.
// NewHost 가 조립을 끝낸 뒤 덮어쓰지 않도록 옵션 적용 순서와 무관하게 동작한다.
func WithService(capability extv1.Capability, svc any) Option {
	return func(h *Host) {
		if svc == nil {
			return
		}
		h.extra = append(h.extra, serviceEntry{capability: capability, svc: svc})
	}
}

// serviceEntry 는 WithService 로 추가된 항목이다.
type serviceEntry struct {
	capability extv1.Capability
	svc        any
}
