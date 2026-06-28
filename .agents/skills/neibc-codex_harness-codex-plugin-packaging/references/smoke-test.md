# Smoke Test 패턴

프로젝트 루트의 `tests/smoke.sh`가 검증할 항목. 기본값은 파일 구조와 JSON 유효성만 확인하고, Codex 설정을 변경하는 설치 검증은 `RUN_CODEX_INSTALL=1`일 때만 수행한다.

```bash
#!/usr/bin/env bash
set -euo pipefail

PLUGIN_NAME="${PLUGIN_NAME:-codex-harness}"
MARKETPLACE_NAME="${MARKETPLACE_NAME:-codex-harness-marketplace}"
PLUGIN_ROOT="${PLUGIN_ROOT:-plugins/${PLUGIN_NAME}}"
RUN_CODEX_INSTALL="${RUN_CODEX_INSTALL:-0}"
RUN_MODEL_SMOKE="${RUN_MODEL_SMOKE:-0}"

echo "[1/6] codex 버전 확인"
codex --version

echo "[2/6] marketplace 구조 확인"
test -f .agents/plugins/marketplace.json
python3 -m json.tool .agents/plugins/marketplace.json >/dev/null
grep -q "\"name\": \"${PLUGIN_NAME}\"" .agents/plugins/marketplace.json

echo "[3/6] plugin manifest 확인"
test -f "${PLUGIN_ROOT}/.codex-plugin/plugin.json"
python3 -m json.tool "${PLUGIN_ROOT}/.codex-plugin/plugin.json" >/dev/null
grep -q "\"name\": \"${PLUGIN_NAME}\"" "${PLUGIN_ROOT}/.codex-plugin/plugin.json"
test -d "${PLUGIN_ROOT}/skills"

echo "[4/6] MCP 팀 서버 확인"
if [[ -f "${PLUGIN_ROOT}/mcp-team-server/dist/index.js" ]]; then
  echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' \
    | timeout 5 node "${PLUGIN_ROOT}/mcp-team-server/dist/index.js" \
    | grep -q team_create || { echo "FAIL: team_create tool missing"; exit 1; }
else
  echo "SKIP: ${PLUGIN_ROOT}/mcp-team-server/dist/index.js not found"
fi

echo "[5/6] Codex marketplace/install 확인"
if [[ "${RUN_CODEX_INSTALL}" == "1" ]]; then
  codex plugin marketplace add "$(pwd)" --json >/dev/null
  codex plugin add "${PLUGIN_NAME}@${MARKETPLACE_NAME}" --json >/dev/null
  codex plugin list --available --json | grep -q "\"pluginId\": \"${PLUGIN_NAME}@${MARKETPLACE_NAME}\""
else
  echo "SKIP: set RUN_CODEX_INSTALL=1 to mutate local Codex plugin config"
fi

echo "[6/6] codex exec dry-run"
if [[ "${RUN_MODEL_SMOKE}" == "1" ]]; then
  codex exec "${PLUGIN_NAME} smoke test: print OK" --max-tokens 100 \
    || { echo "WARN: codex exec failed - check API/auth"; }
else
  echo "SKIP: set RUN_MODEL_SMOKE=1 to run a model-backed smoke"
fi

echo "smoke.sh passed"
```

`RUN_CODEX_INSTALL=1`은 로컬 Codex plugin 설정을 변경한다. CI 또는 안전한 dry-run에서는 기본값 `0`을 유지한다.

`RUN_MODEL_SMOKE=1`은 모델 호출이라 비용이 발생할 수 있다. CI에서는 기본값 `0`을 유지한다.

## 통과 기준

- 1~4: 모두 PASS 또는 명시적 SKIP이어야 한다.
- 5: `RUN_CODEX_INSTALL=1`일 때 PASS 필수.
- 6: 권장. 실패 시 WARN으로 표시하되 구조 검증 자체는 통과 처리 가능.

## 실패 분류

| 단계 | 실패 원인 후보 | 해결 |
|------|--------------|------|
| 1 | codex 미설치 | `npm i -g @openai/codex` |
| 2 | marketplace 파일 누락 또는 JSON 오류 | `.agents/plugins/marketplace.json` 생성/수정 |
| 3 | plugin manifest 누락 또는 JSON 오류 | `.codex-plugin/plugin.json` 생성/수정 |
| 4 | 팀 서버 빌드 오류 또는 SDK 버전 mismatch | `npm install && npm run build` |
| 5 | marketplace 미등록 또는 plugin selector 오류 | `codex plugin marketplace list --json`, `codex plugin list --available --json` 확인 |
| 6 | 인증/API 문제 | `codex login` |
