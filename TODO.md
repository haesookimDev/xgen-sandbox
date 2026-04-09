# xgen-sandbox 추가 개발 TODO

## CRITICAL

- [x] #1 runtime-base Dockerfile에서 무제한 sudo 제거 (`runtime/base/Dockerfile`)

## HIGH

- [x] #2 ResourceSpec API 구현 — CreatePod에서 사용자 요청 리소스 적용 (`agent/internal/k8s/pod_manager.go`)
- [ ] #3 Python/Go SDK에 execStream, openTerminal 구현 (`sdks/python/`, `sdks/go/`)
- [x] #4 Dashboard 터미널 탭에 SandboxTerminal 컴포넌트 통합 (`dashboard/src/app/dashboard/sandboxes/[id]/page.tsx`)
- [ ] #5 Agent 상태 persistence — 재시작 시 sandbox 추적 정보 복구 (`agent/internal/sandbox/manager.go`)
- [x] #6 Helm values.yaml 시크릿을 K8s Secret으로 분리 (`deploy/helm/xgen-sandbox/`)
- [x] #7 Agent Deployment에 securityContext 추가 (`deploy/helm/xgen-sandbox/templates/agent-deployment.yaml`)
- [ ] #8 E2E 테스트 추가 (`.github/workflows/ci.yml`)
- [x] #9 CI에 컨테이너 보안 스캐닝(Trivy) + dashboard-lint 추가 (`.github/workflows/ci.yml`)
- [x] #10 핵심 모듈 테스트 추가 — admin 엔드포인트 테스트 (`agent/internal/server/admin_test.go`)

## MEDIUM

- [ ] #11 모든 SDK에 WebSocket 재연결 로직 추가
- [ ] #12 Dashboard 생성 폼에 env/metadata/resources UI 추가
- [ ] #13 Warm Pool 템플릿별 크기 설정 지원
- [ ] #14 RBAC ClusterRole을 namespace-scoped Role로 축소
- [ ] #15 Rate limit을 환경변수로 설정 가능하게 변경
- [ ] #16 릴리스 자동화 — SemVer 태깅, CHANGELOG 생성
- [ ] #17 CI에 SDK 빌드/테스트 추가
- [ ] #18 모니터링 기본 활성화 및 알림 규칙 보강
- [ ] #19 트러블슈팅 가이드, SDK 기능 매트릭스 문서 작성
- [ ] #20 Docker Compose 기반 로컬 개발 환경 추가
- [ ] #21 API 입력 검증 강화 — 포트 범위, 메타데이터 크기, 중복 포트

## LOW

- [ ] #22 미사용 프로토콜 메시지 타입 정리
- [ ] #23 파일 워칭 polling에서 inotify로 전환
- [ ] #24 OpenTelemetry 분산 트레이싱 통합
- [ ] #25 HPA 커스텀 메트릭 기반 스케일링
- [ ] #26 Pod anti-affinity 규칙 추가
- [ ] #27 성능 벤치마크 문서화
- [ ] #28 핫 리로드 개발환경 (air + volume mount)
- [ ] #29 Makefile help 타겟 추가
