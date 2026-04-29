# xgen-sandbox LLM/성능/기능 안정화 TODO

이 문서는 2026-04-27 전체 프로젝트 분석 결과를 기준으로, 성능/기능 리스크와 LLM 친화 인터페이스 개선을 실제 개발 단위로 쪼갠 작업 목록이다.

## 진행 원칙

- P0는 현재 기능이 깨질 수 있는 버그와 리소스 누수 가능성부터 처리한다.
- P1은 LLM/SDK가 안정적으로 샌드박스를 반복 호출할 수 있게 만드는 인터페이스와 운영 한계를 다룬다.
- P2는 품질, 문서, 관측성, 배포 편의성을 보강한다.
- 각 항목은 가능하면 회귀 테스트 또는 최소 빌드/테스트 검증을 같이 추가한다.

## P0 - 기능 정확성/세션 안정성

- [x] P0-01 Sidecar WebSocket 이벤트를 연결별로 격리한다.
  - 문제: `sidecar/internal/ws/server.go`가 전역 `active` 연결로 파일/포트 이벤트를 보내 여러 클라이언트가 붙으면 이벤트가 마지막 연결로 섞일 수 있다.
  - 작업:
    - connection-local `connState`를 `envelope`에 전달한다.
    - watcher/port detector callback이 반드시 자기 connection으로만 응답하게 한다.
    - `Ping`, `Error`, `FsWatch`도 전역 active fallback 없이 요청 connection에 응답하게 한다.
    - connection별 write mutex를 두어 여러 goroutine의 동시 write를 직렬화한다.
  - 검증:
    - sidecar 단위 테스트로 channel/process fallback을 검증한다.
    - 두 connection의 FsEvent/PortEvent 교차 전송 방지 E2E는 P2 protocol compatibility 테스트에 포함한다.

- [x] P0-02 Terminal stdin/resize/signal의 process id 처리 방식을 고친다.
  - 문제: sidecar는 `process_id`를 요구하지만 SDK/브라우저 컴포넌트는 process id 0 또는 미포함 payload를 보낸다.
  - 작업:
    - sidecar가 `ExecStart` ACK에 포함한 `process_id`와 channel을 session map에 저장한다.
    - `ExecStdin`, `ExecResize`, `ExecSignal`에서 `process_id == 0`이면 channel로 process를 찾는 backward-compatible fallback을 둔다.
    - process 종료 시 channel mapping을 제거한다.
    - TypeScript/Python/Go SDK는 ACK의 `process_id`를 저장해서 stdin/resize/signal에 포함하게 한다.
    - dashboard/browser terminal처럼 process id를 보내지 않는 기존 클라이언트는 sidecar의 channel fallback으로 호환한다.
  - 검증:
    - sidecar handler 단위 테스트 또는 SDK-level smoke 테스트로 terminal input/resize가 실제 process에 전달되는지 확인한다.

- [x] P0-03 Streaming exec exit code 필드명을 SDK 전반에서 통일한다.
  - 문제: sidecar는 `exit_code`를 보내는데 Python/Go streaming SDK는 `exitCode`를 읽는 경로가 있다.
  - 작업:
    - Python SDK `exec_stream`에서 `exit_code` 우선, `exitCode` fallback으로 읽는다.
    - Go SDK `ExecStream` msgpack tag를 `exit_code`로 수정하고 fallback을 둔다.
    - Rust/TypeScript와 protocol docs가 같은 이름을 쓰는지 확인한다.
  - 검증:
    - SDK별 protocol decode 테스트 추가.

- [x] P0-04 Warm pool claim lifecycle을 실제 K8s Pod 상태와 일치시킨다.
  - 문제: 현재 warm pod claim은 agent 캐시만 `warm-* -> sandboxID`로 바꾸고 실제 Pod 이름/라벨/annotation은 바꾸지 않는다. 삭제/복구/watch 이벤트가 새 sandbox ID와 불일치할 수 있다.
  - 작업:
    - warm pod claim 시 K8s Pod label/annotation을 실제 sandbox state로 patch한다.
    - `PodInfo.PodName`을 삭제/force-delete 경로에서 사용해 Pod 이름이 sandbox ID와 달라도 삭제되게 한다.
    - watcher가 label 변경 이벤트를 받으면 old warm ID cache를 제거하고 new sandbox ID cache로 등록하게 한다.
    - warm pool pod와 claimed sandbox를 prefix가 아닌 명시 label/annotation으로 구분한다.
    - browser/gui warm pool은 VNC container 유무가 claim request와 일치하는 경우에만 claim되게 한다.
  - 검증:
    - fake K8s client 테스트로 claim 후 label/annotation/cache/delete가 일관적인지 확인한다.

- [x] P0-05 Agent-side stale sidecar connection을 정리한다.
  - 문제: ready 시 장기 sidecar WS를 열지만 REST exec/client WS는 임시 connection을 사용해 장기 connection이 거의 쓰이지 않는다.
  - 작업:
    - 사용하지 않는 `ConnectToSidecar` 장기 connection을 제거하거나 health/read pump 용도로 명확히 바꾼다.
    - warm pool readiness 확인만 필요하면 HTTP ready probe 또는 short-lived WS로 대체한다.
  - 검증:
    - sandbox create/delete 반복 시 goroutine/connection 누수가 없는지 테스트한다.

## P1 - LLM 친화 SDK/CLI/세션 관리

- [x] P1-01 API/SDK 기본 경로를 v2로 전환한다.
  - TypeScript/Python/Go/Rust SDK와 dashboard가 `/api/v2`를 기본으로 사용하게 한다.
  - v1 compatibility mode를 옵션으로만 남긴다.
  - `timeout_ms`, `created_at_ms`, `expires_at_ms`, structured error를 SDK 타입에 반영한다.

- [x] P1-02 LLM용 CLI를 추가한다.
  - 명령:
    - `xgen auth token --json`
    - `xgen create --template nodejs --ttl-ms 1800000 --metadata key=value --json`
    - `xgen exec <sandbox-id> --json --timeout-ms 30000 --max-output-bytes 65536 -- <cmd>`
    - `xgen exec <sandbox-id> --stream --jsonl -- <cmd>`
    - `xgen fs read|write|list|rm`
    - `xgen port wait <sandbox-id> <port> --timeout-ms ... --json`
    - `xgen session list|get|keepalive|destroy|gc --json`
  - 출력:
    - 기본은 machine-readable JSON/JSONL.
    - 실패 시 `code`, `message`, `retryable`, `details`, `sandbox_id`, `command_id`를 포함한다.

- [ ] P1-03 로컬 session registry를 만든다.
  - 저장 필드:
    - `session_id`, `sandbox_id`, `template`, `cwd`, `ports`, `capabilities`, `created_at_ms`, `expires_at_ms`, `last_used_at_ms`, `metadata`.
  - 자동 keepalive와 idle TTL 정책을 CLI/SDK에서 공유한다.
  - `xgen session gc`가 만료/고아 sandbox를 정리한다.

- [ ] P1-04 LLM-safe exec API를 SDK에 추가한다.
  - `max_output_bytes`, `max_stdout_bytes`, `max_stderr_bytes`를 지원한다.
  - truncation marker와 `truncated: true`를 응답에 포함한다.
  - 큰 출력은 workspace artifact path로 저장하고 preview/read 링크를 반환한다.
  - command cancellation과 timeout error를 안정적으로 표준화한다.

- [ ] P1-05 Codex/Claude/OpenAI용 skill 문서를 추가한다.
  - `skills/xgen-sandbox/SKILL.md` 또는 배포 가능한 skill 템플릿을 만든다.
  - 규칙:
    - CLI는 항상 `--json` 또는 `--jsonl` 사용.
    - 긴 작업은 stream 모드 사용.
    - 큰 출력은 artifact file로 회수.
    - 세션은 metadata에 agent/task id를 남기고 마지막에 destroy/gc.

## P1 - 리소스/성능 최적화

- [ ] P1-06 REST exec output 누적 메모리를 제한한다.
  - agent `ExecSync`가 stdout/stderr를 무제한 string concat하지 않게 bytes.Buffer + limit으로 변경한다.
  - limit 초과 시 process kill 또는 output truncation 정책을 명확히 한다.

- [ ] P1-07 파일 API를 대용량 친화적으로 확장한다.
  - chunked read/write, range read, directory pagination을 추가한다.
  - read/write 최대 크기와 명확한 error code를 둔다.

- [ ] P1-08 WS proxy logging을 debug/trace로 낮춘다.
  - 모든 frame log를 기본 info에서 제거한다.
  - 요청 id/session id 단위 요약 metric만 남긴다.

- [ ] P1-09 Rate limit key를 정규화한다.
  - `RemoteAddr`에서 host만 추출한다.
  - `X-Forwarded-For`는 첫 IP만 사용한다.
  - API key/subject 기반 limit 옵션을 추가한다.

## P2 - GUI/브라우저/문서/운영 품질

- [ ] P2-01 GUI/VNC 구조를 재검토한다.
  - runtime exec가 VNC 화면과 같은 X server를 사용하는지 확인한다.
  - `DISPLAY=:99`와 예제 `DISPLAY=:0` 불일치를 수정한다.
  - GUI sandbox의 runtime/vnc container 책임을 문서와 일치시킨다.

- [ ] P2-02 docs/security.md와 실제 Pod security context를 동기화한다.
  - sidecar가 root + capabilities를 갖는 실제 구현을 문서에 정확히 반영한다.
  - capability별 보안 영향과 운영 권고를 추가한다.

- [ ] P2-03 audit log의 v2/WS coverage를 보강한다.
  - `/api/v2/*` sandbox id extraction을 지원한다.
  - 실제 route pattern 기반으로 action cardinality를 낮춘다.

- [ ] P2-04 CI에 protocol compatibility 테스트를 추가한다.
  - sidecar protocol fixture를 만들고 TS/Python/Go/Rust SDK decode 테스트를 같은 fixture로 검증한다.
  - Terminal ACK/process id fixture를 포함한다.

- [ ] P2-05 warm pool과 startup latency benchmark를 자동화한다.
  - cold/warm start, exec latency, file I/O latency를 CI nightly 또는 local target으로 측정한다.
  - docs/performance.md를 실제 측정값 기준으로 갱신한다.

## 완료된 이전 주요 항목

- [x] runtime-base Dockerfile에서 무제한 sudo 제거
- [x] ResourceSpec API 구현
- [x] Python/Go SDK에 execStream, openTerminal 구현
- [x] Dashboard 터미널 탭 통합
- [x] Agent 상태 persistence 및 K8s pod 복구
- [x] Helm secret 분리 및 agent securityContext 추가
- [x] Kind 기반 E2E 테스트와 CI 보안 스캔 추가
- [x] Warm Pool 템플릿별 크기 설정 지원
- [x] SDK WebSocket 재연결 로직 추가
