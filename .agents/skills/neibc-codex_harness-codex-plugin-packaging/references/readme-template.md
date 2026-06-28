# 루트 README.md 템플릿

빌더 에이전트가 Codex native marketplace 구조의 프로젝트 루트 README.md를 갱신할 때 사용할 골격.

전제:
- repo 루트에는 `.agents/plugins/marketplace.json`이 있다.
- 실제 플러그인은 `plugins/codex-harness/` 아래에 있다.
- 플러그인 매니페스트는 `plugins/codex-harness/.codex-plugin/plugin.json`이다.

````markdown
# codex-harness

Agent Team & Skill Architect - `revfactory/harness` Claude Code 플러그인의 **Codex CLI 포트**.

## 무엇이 다른가

| Claude Code 원본 | Codex 포트 |
|------------------|-----------|
| Skill 자동 트리거 | Codex skill description 기반 트리거 + 명시 요청 |
| Agent Team primitive | MCP 팀 서버 (`mcp-team-server/`) |
| `Agent` 도구 | Codex 세션/서브프로세스 또는 MCP 팀 서버 |
| `CLAUDE.md` | `AGENTS.md` |
| `.claude-plugin/marketplace.json` | `.agents/plugins/marketplace.json` |

> **Known limitations**: 메시지 전달이 폴링 기반(즉시 wake 없음), 자동 컨텍스트 압축 미지원, WebFetch/WebSearch는 별도 MCP 서버 필요. 자세한 내용은 `LIMITATIONS.md` 참조.

## 설치

```bash
# 1. 클론
git clone <repo>
cd codex_harness

# 2. MCP 팀 서버 빌드
cd plugins/codex-harness/mcp-team-server
npm install && npm run build
cd ../../..

# 3. Codex marketplace 등록 및 플러그인 설치
codex plugin marketplace add "$(pwd)"
codex plugin add codex-harness@codex-harness-marketplace
```

## 사용

```bash
# 인터랙티브
codex
> codex-harness 스킬을 사용해서 도메인 분석 후 하네스 구성해줘

# 또는 비대화형
codex exec "codex-harness 스킬을 사용해서 도메인 분석 후 하네스 구성해줘"
```

## 동작 확인

```bash
./tests/smoke.sh
```

## Troubleshooting

- "plugin not found" -> `codex plugin list --available --json`에서 `codex-harness@codex-harness-marketplace`가 보이는지 확인.
- "marketplace not found" -> `codex plugin marketplace list --json`에서 `codex-harness-marketplace`가 보이는지 확인. 없으면 `codex plugin marketplace add "$(pwd)"` 재실행.
- "team server not found" -> `codex mcp list`에 team 항목이 있는지 확인. 없으면 MCP 설정을 재확인.
- skill 트리거가 안 잡힘 -> 요청에 `codex-harness 스킬을 사용해서`처럼 skill 이름을 명시.
- 폴링이 멈춤 -> `~/.codex/teams.sqlite`의 task 상태 확인. 필요 시 수동 정리.

## 라이선스

Apache-2.0 (원본 동일)
````
