# Smoke Test 패턴

프로젝트 루트의 `tests/smoke.sh`가 검증할 항목 (빌더가 placeholder를 이 내용으로 갱신).

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "[1/5] codex 버전 확인"
codex --version

echo "[2/5] 플러그인 등록 상태"
codex mcp list | grep -q team || { echo "FAIL: team MCP server not registered"; exit 1; }

echo "[3/5] 프롬프트 파일 존재"
test -f prompts/harness.md
test -f prompts/codex-harness-orchestrator.md
test -d agents

echo "[4/5] 팀 서버 stdio handshake"
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' \
  | timeout 5 node mcp-team-server/dist/index.js \
  | grep -q team_create || { echo "FAIL: team_create tool missing"; exit 1; }

echo "[5/5] codex exec dry-run"
codex exec --prompt-file prompts/harness.md "smoke test: print 'OK'" \
  --max-tokens 100 \
  || { echo "WARN: codex exec failed — check API/auth"; }

echo "✅ smoke.sh passed"
```

> Step 5는 모델 호출이라 비용 발생 가능. CI에서는 환경변수로 옵션화.

## 통과 기준

- 1~4: 모두 PASS 필수 (구조적 검증)
- 5: 권장. 실패 시 WARN으로 표시하되 빌드 자체는 통과 처리 가능.

## 실패 분류

| 단계 | 실패 원인 후보 | 해결 |
|------|--------------|------|
| 1 | codex 미설치 | `npm i -g @openai/codex` |
| 2 | MCP 등록 누락 | `codex mcp add team ...` 재실행 |
| 3 | 빌드 산출물 누락 | `codex-plugin-builder` 재호출 |
| 4 | 팀 서버 빌드 오류 또는 SDK 버전 mismatch | `npm install && npm run build` |
| 5 | 인증/API 문제 | `codex login` |
