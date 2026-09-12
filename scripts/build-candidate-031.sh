#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

candidate_version="${CANDIDATE_VERSION:-0.31.0}"
[[ "$candidate_version" == "0.31.0" ]] || { echo "unexpected candidate version: $candidate_version" >&2; exit 2; }
[[ "$(tr -d '\r\n' < VERSION)" == "$candidate_version" ]] || { echo "source VERSION must match the exact promoted candidate version" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "python3 is required to generate candidate SBOM" >&2; exit 2; }

commit="${CANDIDATE_SHA:-$(git rev-parse HEAD)}"
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || { echo "candidate SHA must be exact 40-char commit" >&2; exit 2; }
source_date_epoch="${SOURCE_DATE_EPOCH:-$(git show -s --format=%ct "$commit")}"
[[ "$source_date_epoch" =~ ^[0-9]+$ ]] || { echo "invalid SOURCE_DATE_EPOCH" >&2; exit 2; }
build_time="$(date -u -d "@$source_date_epoch" +%Y-%m-%dT%H:%M:%SZ)"

dist_dir="${DIST_DIR:-$repo_root/dist/candidate-0.31.0}"
rm -rf "$dist_dir"
mkdir -p "$dist_dir"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
bundle="control-center-$candidate_version"
mkdir -p "$stage/$bundle/bin" "$stage/$bundle/api" "$stage/$bundle/config" "$stage/$bundle/deploy/systemd" "$stage/$bundle/migrations" "$stage/$bundle/scripts" "$stage/$bundle/docs"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath -buildvcs=false \
  -ldflags="-s -w -X control-center/internal/buildinfo.Version=$candidate_version -X control-center/internal/buildinfo.Commit=$commit -X control-center/internal/buildinfo.BuildTime=$build_time" \
  -o "$stage/$bundle/bin/control-center" ./cmd/control-center

cp -a api/. "$stage/$bundle/api/"
cp config/control-center.env.example "$stage/$bundle/config/"
cp deploy/systemd/control-center.service "$stage/$bundle/deploy/systemd/"
find migrations -maxdepth 1 -type f \( -name '*.sql' -o -name 'README.md' \) -exec cp {} "$stage/$bundle/migrations/" \;
cp scripts/migrate.sh "$stage/$bundle/scripts/"
for doc in docs/RELEASE_0.31.0_RU.md docs/RELEASE_CANDIDATE_READINESS_0.31_RU.md docs/QUALIFICATION_0.31_UPGRADE_FROM_0.30_RU.md docs/QUALIFICATION_0.31_RELEASE_PROMOTION_GATE_RU.md; do
  [[ -f "$doc" ]] && cp "$doc" "$stage/$bundle/docs/"
done

# Third-party evidence is part of the distributable candidate, not an external
# mutable lookup. The SBOM generator validates the exact go.mod module set and
# the SHA-256 of every checked-in license text before producing CycloneDX.
cp -a third_party "$stage/$bundle/third_party"
cp THIRD_PARTY_NOTICES.md "$stage/$bundle/THIRD_PARTY_NOTICES.md"
sbom="$dist_dir/control-center-$candidate_version.sbom.cdx.json"
python3 scripts/generate-sbom-031.py third_party/manifest-0.31.json "$sbom" "$commit"
cp "$sbom" "$stage/$bundle/docs/SBOM.cdx.json"
cp THIRD_PARTY_NOTICES.md "$dist_dir/THIRD_PARTY_NOTICES.md"

printf '%s\n' "$candidate_version" > "$stage/$bundle/VERSION"
printf '%s\n' "$commit" > "$stage/$bundle/REVISION"
printf '%s\n' "$build_time" > "$stage/$bundle/BUILD_TIME"
chmod 0755 "$stage/$bundle/bin/control-center" "$stage/$bundle/scripts/migrate.sh"

artifact="$dist_dir/control-center-$candidate_version-linux-amd64.tar.gz"
tar --sort=name --mtime="@$source_date_epoch" --owner=0 --group=0 --numeric-owner -C "$stage" -cf - "$bundle" | gzip -n > "$artifact"
(
  cd "$dist_dir"
  sha256sum "$(basename "$artifact")" > "$(basename "$artifact").sha256"
)

source_artifact="$dist_dir/control-center-$candidate_version-source.tar.gz"
git archive --format=tar --prefix="control-center-$candidate_version-source/" "$commit" | gzip -n > "$source_artifact"

printf 'CANDIDATE_VERSION=%s\nCANDIDATE_SHA=%s\nBUILD_TIME=%s\nBINARY_ARTIFACT=%s\nSOURCE_ARTIFACT=%s\nSBOM=%s\nTHIRD_PARTY_NOTICES=%s\n' \
  "$candidate_version" "$commit" "$build_time" "$artifact" "$source_artifact" "$sbom" "$dist_dir/THIRD_PARTY_NOTICES.md"
