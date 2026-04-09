# xgen-sandbox 추가 개발 TODO

## CRITICAL

- [x] #1 runtime-base Dockerfile에서 무제한 sudo 제거 (`runtime/base/Dockerfile`)

## HIGH

- [x] #2 ResourceSpec API 구현 — CreatePod에서 사용자 요청 리소스 적용 (`agent/internal/k8s/pod_manager.go`)
- [x] #3 Python/Go SDK에 execStream, openTerminal 구현 (`sdks/python/`, `sdks/go/`)
- [x] #4 Dashboard 터미널 탭에 SandboxTerminal 컴포넌트 통합 (`dashboard/src/app/dashboard/sandboxes/[id]/page.tsx`)
- [x] #5 Agent 상태 persistence — K8s pod 스캔으로 재시작 시 복구 (`agent/internal/k8s/pod_manager.go`)
- [x] #6 Helm values.yaml 시크릿을 K8s Secret으로 분리 (`deploy/helm/xgen-sandbox/`)
- [x] #7 Agent Deployment에 securityContext 추가 (`deploy/helm/xgen-sandbox/templates/agent-deployment.yaml`)
- [x] #8 E2E 테스트 추가 — Kind 기반 전체 플로우 검증 (`.github/workflows/e2e.yml`)
- [x] #9 CI에 컨테이너 보안 스캐닝(Trivy) + dashboard-lint 추가 (`.github/workflows/ci.yml`)
- [x] #10 핵심 모듈 테스트 추가 — admin 엔드포인트 테스트 (`agent/internal/server/admin_test.go`)

## MEDIUM

- [x] #11 모든 SDK에 WebSocket 재연결 로직 추가 (TS + Python + Go)
- [x] #12 Dashboard 생성 폼에 env/metadata UI 추가 (`dashboard/src/app/dashboard/sandboxes/page.tsx`)
- [x] #13 Warm Pool 템플릿별 크기 설정 지원 — `WARM_POOL_SIZES=base:3,nodejs:2` (`agent/internal/k8s/warm_pool.go`)
- [x] #14 RBAC ClusterRole을 namespace-scoped Role로 축소 (`deploy/helm/xgen-sandbox/templates/rbac.yaml`)
- [x] #15 Rate limit을 환경변수로 설정 가능하게 변경 — `RATE_LIMIT_PER_MINUTE` (`agent/internal/config/config.go`)
- [x] #16 릴리스 자동화 — SemVer 태깅, GitHub Release, 버전 이미지 (`.github/workflows/release.yml`)
- [x] #17 CI에 SDK 빌드/테스트 추가 — TypeScript, Go, Python (`.github/workflows/ci.yml`)
- [x] #18 모니터링 알림 규칙 보강 — Agent 재시작, Pod 생성 실패 알림 추가 (`deploy/helm/xgen-sandbox/templates/prometheusrule.yaml`)
- [x] #19 트러블슈팅 가이드 + SDK 기능 매트릭스 문서 (`docs/troubleshooting.md`, `docs/sdk-feature-matrix.md`)
- [x] #20 Docker Compose 기반 로컬 개발 환경 추가 (`docker-compose.yml`)
- [x] #21 API 입력 검증 강화 — 포트 범위, 메타데이터 크기, 중복 포트 (`agent/internal/server/http.go`)

## LOW

- [x] #22 미사용 프로토콜 메시지 타입에 reserved 주석 추가 (`agent/pkg/protocol/messages.go`)
- [x] #23 파일 워칭 — fsnotify 전환 가이드 주석 추가 (`sidecar/internal/fs/watcher.go`)
- [x] #24 OpenTelemetry — 트레이싱 미들웨어 계장 포인트 준비 (`agent/internal/server/http.go`)
- [x] #25 HPA 메모리 기반 스케일링 추가 (`deploy/helm/xgen-sandbox/templates/hpa.yaml`)
- [x] #26 Pod anti-affinity 규칙 추가 (`deploy/helm/xgen-sandbox/templates/agent-deployment.yaml`)
- [x] #27 성능 벤치마크 문서화 (`docs/performance.md`)
- [x] #28 핫 리로드 개발환경 — air 설정 + Makefile 타겟 (`.air.toml`, `Makefile`)
- [x] #29 Makefile help 타겟 추가 (`Makefile`)
