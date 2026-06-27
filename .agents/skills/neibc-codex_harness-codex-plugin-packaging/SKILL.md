---
name: codex-plugin-packaging
description: Codex CLI 플러그인 매니페스트와 마켓 등록 형식, 디렉토리 구조, 배포 절차를 다루는 스킬. 프로젝트 루트가 곧 Codex 플러그인이 되도록 plugin.toml 작성, codex plugin marketplace 명령 사용, 사용자 설치 절차(codex plugin install . 또는 git URL 직접 설치) 안내가 필요할 때 반드시 사용. revfactory의 .claude-plugin/marketplace.json 형식을 Codex 호환으로 변환할 때도 호출.
---

# Codex Plugin Packaging

Codex 플러그인의 정확한 매니페스트 형식과 배포 절차. 형식은 `codex plugin marketplace --help` 실측치를 기준으로 한다.

## 1. 표준 디렉토리 구조 (프로젝트 루트 = Codex 플러그인)

```
codex_harness/                       # ← git 저장소 루트 = Codex 플러그인 루트
├── README.md                        # 사용자 진입점 (Claude/Codex 모두용)
├── LICENSE
├── .gitignore
├── CLAUDE.md                        # Claude Code 진입 지침 (Codex 무시)
├── AGENTS.md                        # Codex 진입 지침
├── plugin.toml                      # Codex 플러그인 매니페스트 (실측 schema 반영)
├── LIMITATIONS.md                   # 변환 손실 정리
├── prompts/                         # 슬래시/명시 호출 프롬프트
│   ├── harness.md                   # 메인 트리거
│   ├── codex-harness-orchestrator.md
│   └── ...
├── agents/                          # 에이전트 페르소나
│   ├── codex-internals-analyst.md
│   ├── claude-harness-cartographer.md
│   ├── primitive-translator.md
│   ├── codex-plugin-builder.md
│   └── codex-harness-qa.md
├── mcp-team-server/                 # 팀 에뮬레이션 MCP 서버
│   ├── package.json
│   ├── tsconfig.json
│   ├── src/
│   └── README.md
├── hooks/                           # (Codex가 hooks 미지원 시 shell wrapper)
├── tests/
│   └── smoke.sh
├── .claude/                         # ── Claude Code 개발 하네스 (Codex 무시) ──
│   ├── agents/
│   └── skills/
└── _workspace/                      # gitignored — 중간 산출물
```

> 핵심: `dist/` 같은 서브트리에 **빌드 출력하지 않는다**. 저장소 루트가 곧 설치 가능한 Codex 플러그인이며, 같은 저장소가 동시에 Claude Code 측 개발 하네스를 `.claude/` 아래에 보유한다.

## 2. 매니페스트 형식

> ⚠️ Codex 플러그인 매니페스트의 정확한 schema(키 이름, 필수 필드, 마켓 등록 절차)는 **`codex-internals-analyst`가 `codex plugin marketplace --help` 실측 결과**를 `_workspace/01_codex_primitives.md`에 기록한 다음 확정한다. 아래는 그 결과가 반영되기 전의 스켈레톤.

### TOML 추정 스켈레톤

```toml
# plugin.toml
name = "codex-harness"
version = "0.1.0"
description = "Agent Team & Skill Architect — Codex port of revfactory/harness"
author = "..."
license = "Apache-2.0"

[[prompts]]
name = "harness"
file = "prompts/harness.md"
trigger = "/harness"

[[prompts]]
name = "codex-harness-orchestrator"
file = "prompts/codex-harness-orchestrator.md"

[mcp_servers.team]
command = "node"
args = ["./mcp-team-server/dist/index.js"]

[agents]
directory = "agents/"
```

→ 실측 후 키 이름/구조 보정.

## 3. 마켓 등록

```bash
# 사용자 측 설치 (예상 흐름)
codex plugin marketplace add <git-url-or-local-path>

# 또는 직접 등록
codex plugin install .
```

정확한 명령과 marketplace.json 형식은 분석 결과로 확정.

## 4. revfactory marketplace.json → Codex marketplace 매니페스트

원본 (Claude Code):
```json
{
  "name": "harness-marketplace",
  "owner": { "name": "revfactory", "url": "https://github.com/revfactory" },
  "plugins": [
    { "name": "harness", "source": "./", "description": "...", "version": "1.1.0" }
  ]
}
```

Codex 측 (가설):
```toml
# .codex-marketplace.toml (또는 동등 파일)
name = "harness-marketplace"
owner = "revfactory"
homepage = "https://github.com/revfactory"

[[plugins]]
name = "codex-harness"
source = "./"
description = "Agent Team & Skill Architect (Codex port)"
version = "0.1.0"
```

실측 후 보정.

## 5. 첫 빌드 acceptance criteria

빌더 에이전트가 다음을 만족하면 빌드 성공:
1. `codex plugin install .` (또는 등가 명령)이 에러 없이 종료
2. `codex` 진입 후 `/harness` (또는 명시 호출)로 prompt 트리거
3. `codex mcp list`에서 팀 서버 보임
4. `tests/smoke.sh` 통과

## 6. 배포 채널

- **GitHub repo**: revfactory/harness 저장소의 `codex/` 서브디렉토리 또는 `harness-codex` 별도 저장소 — 사용자가 `codex plugin marketplace add` 가능
- **로컬 개발**: 프로젝트 루트에서 `codex plugin install .`
- **npm 패키지(추후)**: 별도 검토 (Codex가 npm 마켓을 지원하는지에 따라)

## 참조

- 매니페스트 schema 실측 결과는 `_workspace/01_codex_primitives.md` 섹션 "플러그인 시스템"
- README 템플릿은 `references/readme-template.md`
- smoke 테스트 패턴은 `references/smoke-test.md`
