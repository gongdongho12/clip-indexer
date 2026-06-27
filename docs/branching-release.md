# Branching and Release Strategy

이 프로젝트는 단순한 trunk-based flow를 사용합니다.

## Branches

- `main`은 항상 릴리즈 가능한 상태로 유지하고 branch protection을 겁니다.
- 기능/수정 작업은 `main`에서 짧은 브랜치를 따서 pull request로 머지합니다.
- 브랜치 prefix는 아래처럼 씁니다:
  - `feat/<short-name>`: 사용자 기능.
  - `fix/<short-name>`: 버그 수정.
  - `docs/<short-name>`: 문서 변경.
  - `chore/<short-name>`: 유지보수.
- `release/vX.Y`는 `main`이 이미 다음 개발로 넘어간 뒤 이전 릴리즈 라인에 patch가 필요할 때만 사용합니다.

## Pull requests

모든 pull request는 머지 전에 CI를 통과해야 합니다. CI는 아래를 실행합니다:

- `gofmt` 확인.
- `go vet ./...`.
- `go test ./...`.
- Linux CLI build와 `--version` smoke test.

## Versioning

릴리즈는 `vMAJOR.MINOR.PATCH` 형식의 SemVer tag를 사용합니다.

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

릴리즈 workflow는 Go linker flags로 tag, commit SHA, build timestamp를 바이너리에 주입합니다. 릴리즈 바이너리는 아래 명령으로 버전을 확인할 수 있습니다:

```bash
clip-indexer --version
```

개발 build의 기본 버전은 `0.1.0-dev`입니다.

## Automatic branch release

자동 배포 릴리즈가 필요하면 `release/auto` 브랜치에 push합니다.

```bash
git push origin HEAD:release/auto
```

`Branch Auto Release` workflow는 먼저 formatting, vet, test를 실행합니다. 통과하면 최신 release tag의 `vX.Y.Z` 버전을 읽고 patch를 1 올려 아래 형식의 새 stable tag를 만듭니다.

```text
vX.Y.Z
```

예를 들어 최신 release tag가 `v0.1.1`이면 다음 자동 tag는 `v0.1.2`처럼 생성됩니다. workflow가 tag를 만든 뒤 같은 tag로 `Release` workflow를 dispatch하고, GitHub Release에는 일반 release로 표시됩니다.

자동 릴리즈 브랜치를 바꾸고 싶으면 `.github/workflows/branch-release.yml`의 `on.push.branches` 값을 수정합니다.

릴리즈 노트는 `.github/scripts/release_notes.py`가 commit subject를 읽어 생성합니다. 기능 추가가 `Features` 섹션에 나오려면 commit 제목을 Conventional Commit 형태로 남깁니다.

```text
feat: add local web file planner
fix: keep analysis cache stable after rename
ci: publish checksum file with release assets
```

`Release Notes` workflow는 `main`에 push될 때 commit subject를 기준으로 `CHANGELOG.md`를 갱신합니다. GitHub Release에 들어가는 릴리즈 노트는 dispatch된 `Release` workflow에서 이전 release tag 이후 commit을 같은 Conventional Commit 분류 규칙으로 다시 생성합니다.

## Release flow

1. 릴리즈할 변경사항을 `main`에 머지합니다.
2. `main`의 CI가 통과했는지 확인합니다.
3. `v0.1.0` 같은 annotated tag를 만들고 push합니다.
4. `Release` workflow가 Linux, macOS, Windows용 archive를 빌드합니다.
5. GitHub Release에 archive, release notes, `SHA256SUMS.txt`가 업로드됩니다.

## Automated release

현재 작업물을 새 patch release로 배포하려면 `main` 또는 현재 검증된 브랜치에서 아래처럼 자동 릴리즈 브랜치로 push합니다.

```bash
git push origin HEAD:release/auto
```

이 push는 `Branch Auto Release` workflow를 실행합니다. workflow는 gofmt, vet, test를 통과한 뒤 릴리즈 노트 preview를 생성하고, 최신 release tag 기준 다음 patch 버전의 `vX.Y.Z` tag를 push합니다. 그 다음 같은 tag로 `Release` workflow를 dispatch해 platform별 archive, checksums, GitHub release notes를 발행합니다.

## Hotfixes

이전 릴리즈 라인에 patch release가 필요하면:

1. 해당 series의 최신 tag에서 `release/vX.Y`를 만듭니다.
2. `release/vX.Y`를 대상으로 pull request를 열어 수정합니다.
3. release branch에서 `vX.Y.1` 같은 patch tag를 만듭니다.
4. 같은 수정사항을 `main`에 merge 또는 cherry-pick합니다.
