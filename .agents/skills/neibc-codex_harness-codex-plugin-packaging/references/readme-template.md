# 루트 README.md — 템플릿 (빌더가 갱신)

빌더 에이전트가 프로젝트 루트의 README.md를 갱신할 때 사용할 골격.

> 루트 README에는 이미 placeholder 버전이 있다 (Claude/Codex 듀얼-네이처 안내). 빌더는 placeholder를 덮어쓰지 않고, "Codex 사용자용 빠른 설치" 섹션과 "Known limitations" 섹션만 실측 결과로 갱신한다.

```markdown
# codex-harness

Agent Team & Skill Architect — `revfactory/harness` Claude Code 플러그인의 **Codex CLI 포트**.

## 무엇이 다른가

| Claude Code 원본 | Codex 포트 |
|------------------|-----------|
| Skill 자동 트리거 | 슬래시 트리거 + 명시 `codex exec --prompt-file` |
| Agent Team primitive | MCP 팀 서버 (`mcp-team-server/`) |
| `Agent` 도구 | `codex exec` 서브프로세스 |
| `CLAUDE.md` | `AGENTS.md` |

> **Known limitations**: 메시지 전달이 폴링 기반(즉시 wake 없음), 자동 컨텍스트 압축 미지원, WebFetch/WebSearch는 별도 MCP 서버 필요. 자세한 내용은 `LIMITATIONS.md` 참조.

## 설치

```bash
# 1. 클론
git clone <repo>
cd codex_harness

# 2. MCP 팀 서버 빌드
cd mcp-team-server
npm install && npm run build
cd ..

# 3. Codex에 등록
codex mcp add team --command node --args "$(pwd)/mcp-team-server/dist/index.js"
codex plugin install .   # 또는 codex plugin marketplace add <url>
```

## 사용

```bash
# 인터랙티브
codex
> /harness 도메인 분석 후 하네스 구성해줘

# 또는 비대화형
codex exec --prompt-file prompts/harness.md "도메인 분석 후 하네스 구성해줘"
```

## 동작 확인

```bash
./tests/smoke.sh
```

## Troubleshooting

- "team server not found" → `codex mcp list`에 team 항목이 있는지 확인. 없으면 `codex mcp add` 재실행.
- 슬래시 커맨드가 안 잡힘 → 비대화형 `codex exec --prompt-file` 으로 우회.
- 폴링이 멈춤 → `~/.codex/teams.sqlite`의 task 상태 확인. 필요 시 수동 정리.

## 라이선스

Apache-2.0 (원본 동일)
```
