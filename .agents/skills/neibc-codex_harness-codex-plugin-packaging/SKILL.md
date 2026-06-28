---
name: codex-plugin-packaging
description: Codex CLI 플러그인 매니페스트와 marketplace 등록 형식, 디렉토리 구조, 배포 절차를 다루는 스킬. .codex-plugin/plugin.json 작성, .agents/plugins/marketplace.json 작성, codex plugin marketplace/add 명령 사용, revfactory의 .claude-plugin/marketplace.json 형식을 Codex native 구조로 변환할 때 반드시 사용.
---

# Codex Plugin Packaging

Codex 플러그인의 매니페스트 형식과 배포 절차. 이 문서는 로컬 `codex-cli 0.142.3`의 `codex plugin --help`, `codex plugin add --help`, `codex plugin marketplace --help` 실측 결과와 설치된 marketplace 파일 구조를 기준으로 한다.

## 1. 표준 디렉토리 구조

Codex native 구조에서는 저장소 루트가 marketplace 루트이고, 실제 플러그인은 `plugins/<plugin-name>/` 아래에 둔다.

```
codex_harness/                       # git 저장소 루트 = Codex marketplace 루트
├── README.md                        # 사용자 진입점
├── LICENSE
├── .gitignore
├── CLAUDE.md                        # Claude Code 진입 지침 (Codex 무시)
├── AGENTS.md                        # Codex 진입 지침
├── LIMITATIONS.md                   # 변환 손실 정리
├── .agents/
│   └── plugins/
│       └── marketplace.json         # Codex marketplace 등록 파일
├── plugins/
│   └── codex-harness/               # Codex 플러그인 루트
│       ├── .codex-plugin/
│       │   └── plugin.json          # Codex 플러그인 매니페스트
│       ├── AGENTS.md                # 플러그인 사용 지침
│       ├── skills/
│       │   └── harness/
│       │       └── SKILL.md
│       ├── prompts/                 # 명시 호출 프롬프트가 필요한 경우
│       │   └── harness.md
│       ├── mcp-team-server/         # 팀 에뮬레이션 MCP 서버가 필요한 경우
│       │   ├── package.json
│       │   ├── tsconfig.json
│       │   └── src/
│       └── .mcp.json                # MCP 서버 연결이 있는 경우
├── tests/
│   └── smoke.sh
└── _workspace/                      # gitignored - 중간 산출물
```

현재 Codex CLI에는 `codex plugin install .` 명령이 없다. 설치 단위는 `codex plugin add <plugin>@<marketplace>`이고, repo/team marketplace는 먼저 `codex plugin marketplace add <SOURCE>`로 등록한다.

## 2. 플러그인 매니페스트

Codex 플러그인 매니페스트는 TOML이 아니라 JSON이다. 플러그인 루트의 `.codex-plugin/plugin.json`에 둔다.

```json
{
  "name": "codex-harness",
  "version": "0.1.0",
  "description": "Agent Team & Skill Architect - Codex port of revfactory/harness",
  "author": {
    "name": "revfactory",
    "url": "https://github.com/revfactory"
  },
  "homepage": "https://github.com/revfactory/harness",
  "repository": "https://github.com/revfactory/harness",
  "license": "Apache-2.0",
  "keywords": ["harness", "agent-team", "skill-architect", "codex"],
  "skills": "./skills/",
  "mcpServers": "./.mcp.json",
  "interface": {
    "displayName": "Codex Harness",
    "shortDescription": "Build project-specific agent teams and skills.",
    "longDescription": "A Codex port of revfactory/harness for generating project-specific agent teams, skills, and orchestration guidance.",
    "developerName": "revfactory",
    "category": "Engineering",
    "capabilities": ["Interactive", "Write"],
    "websiteURL": "https://github.com/revfactory/harness",
    "privacyPolicyURL": "https://github.com/revfactory/harness",
    "termsOfServiceURL": "https://github.com/revfactory/harness"
  }
}
```

규칙:
- `name`은 플러그인 폴더명과 일치시킨다.
- `version`은 semver를 사용한다.
- `skills`, `mcpServers`, `apps`는 실제 파일/디렉토리가 있을 때만 선언한다.
- `hooks`는 검증 도구가 거부할 수 있으므로 기본 매니페스트에는 넣지 않는다.
- URL 필드는 `https://` 절대 URL을 사용한다.
- 매니페스트에는 placeholder를 남기지 않는다.

## 3. Marketplace 등록 파일

Codex marketplace는 `.agents/plugins/marketplace.json`에 JSON으로 둔다.

```json
{
  "name": "codex-harness-marketplace",
  "interface": {
    "displayName": "Codex Harness"
  },
  "plugins": [
    {
      "name": "codex-harness",
      "source": {
        "source": "local",
        "path": "./plugins/codex-harness"
      },
      "policy": {
        "installation": "AVAILABLE",
        "authentication": "ON_INSTALL"
      },
      "category": "Engineering"
    }
  ]
}
```

규칙:
- `plugins[].name`은 플러그인 폴더명 및 `plugin.json`의 `name`과 일치시킨다.
- `source.path`는 marketplace 루트 기준 상대 경로로 둔다.
- `policy.installation`은 `AVAILABLE`, `NOT_AVAILABLE`, `INSTALLED_BY_DEFAULT` 중 하나다.
- `policy.authentication`은 `ON_INSTALL`, `ON_USE` 중 하나다.
- `category`는 항상 포함한다.
- `interface.displayName`은 marketplace 최상위 `interface` 아래에 둔다.

## 4. 설치 흐름

Repo/team marketplace:

```bash
codex plugin marketplace add <repo-root-or-git-url>
codex plugin add codex-harness@codex-harness-marketplace
```

개인 marketplace:

```bash
# ~/.agents/plugins/marketplace.json + ~/plugins/codex-harness/ 구조
codex plugin add codex-harness@personal
```

참고:
- 기본 개인 marketplace는 `~/.agents/plugins/marketplace.json`이며 Codex가 암묵적으로 발견한다.
- 개인 marketplace 흐름에서는 `codex plugin marketplace add`가 필요 없다.
- repo/team marketplace나 Git marketplace는 `codex plugin marketplace add <SOURCE>`로 등록한다.
- `codex plugin list --available --json`으로 설치 가능/설치 상태를 확인한다.
- `codex plugin marketplace list --json`으로 등록된 repo/team marketplace를 확인한다.

## 5. revfactory marketplace.json 변환

원본 Claude Code marketplace:

```json
{
  "name": "harness-marketplace",
  "owner": { "name": "revfactory", "url": "https://github.com/revfactory" },
  "plugins": [
    { "name": "harness", "source": "./", "description": "...", "version": "1.2.0" }
  ]
}
```

Codex native marketplace:

```json
{
  "name": "codex-harness-marketplace",
  "interface": {
    "displayName": "Codex Harness"
  },
  "plugins": [
    {
      "name": "codex-harness",
      "source": {
        "source": "local",
        "path": "./plugins/codex-harness"
      },
      "policy": {
        "installation": "AVAILABLE",
        "authentication": "ON_INSTALL"
      },
      "category": "Engineering"
    }
  ]
}
```

호환성 메모: 현재 로컬 환경에서는 `codex plugin marketplace add https://github.com/revfactory/harness.git`로 기존 `.claude-plugin/marketplace.json` 기반 `harness@harness-marketplace`가 설치되어 있다. 새 Codex 포트를 만들 때는 `.agents/plugins/marketplace.json`과 `.codex-plugin/plugin.json`을 생성하는 native 구조를 우선한다.

## 6. 첫 빌드 acceptance criteria

빌더 에이전트가 다음을 만족하면 빌드 성공:
1. `.agents/plugins/marketplace.json`이 유효한 JSON이고 `codex-harness` entry를 포함한다.
2. `plugins/codex-harness/.codex-plugin/plugin.json`이 유효한 JSON이고 필수 메타데이터와 `skills` 경로를 포함한다.
3. `codex plugin marketplace add <repo-root-or-git-url>`가 repo/team marketplace를 등록한다.
4. `codex plugin add codex-harness@codex-harness-marketplace`가 플러그인을 설치한다.
5. MCP 서버가 있으면 `codex mcp list`에서 연결이 보이고 stdio handshake가 통과한다.
6. `tests/smoke.sh`가 구조 검증을 통과한다.

## 7. 배포 채널

- **GitHub repo/team marketplace**: 사용자가 `codex plugin marketplace add <git-url-or-owner/repo>` 후 `codex plugin add codex-harness@codex-harness-marketplace` 실행.
- **로컬 repo/team marketplace**: 사용자가 `codex plugin marketplace add <repo-root>` 후 `codex plugin add codex-harness@codex-harness-marketplace` 실행.
- **개인 marketplace**: `~/.agents/plugins/marketplace.json` + `~/plugins/codex-harness/` 구조. 이 경우 Codex가 marketplace를 암묵적으로 발견하므로 `codex plugin add codex-harness@personal`만 필요하다.
- **npm 패키지**: 현재 CLI help에는 npm marketplace 설치 흐름이 보이지 않으므로 별도 배포 채널로 문서화하지 않는다.

## 참조

- README 템플릿은 `references/readme-template.md`
- smoke 테스트 패턴은 `references/smoke-test.md`
