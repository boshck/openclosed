#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MIGRATIONS_DIR="${ROOT_DIR}/migrations"

if [[ ! -d "${MIGRATIONS_DIR}" ]]; then
  echo "migrations directory not found: ${MIGRATIONS_DIR}" >&2
  exit 1
fi

mapfile -t versions < <(
  find "${MIGRATIONS_DIR}" -maxdepth 1 -type f -name '*.sql' -printf '%f\n' \
    | sed -E 's/^([0-9]+)_.*/\1/' \
    | sort -u
)

if [[ ${#versions[@]} -eq 0 ]]; then
  echo "no migration files found in ${MIGRATIONS_DIR}" >&2
  exit 1
fi

for version in "${versions[@]}"; do
  mapfile -t files < <(find "${MIGRATIONS_DIR}" -maxdepth 1 -type f -name "${version}_*.sql" -printf '%f\n' | sort)
  up_count="$(printf '%s\n' "${files[@]}" | grep -Ec "^${version}_.+\\.up\\.sql$" || true)"
  down_count="$(printf '%s\n' "${files[@]}" | grep -Ec "^${version}_.+\\.down\\.sql$" || true)"

  if [[ "${up_count}" != "1" || "${down_count}" != "1" || "${#files[@]}" != "2" ]]; then
    echo "migration numbering check failed for version ${version}:" >&2
    printf '%s\n' "${files[@]}" | sed 's/^/  - /' >&2
    echo "expected exactly one .up.sql and one .down.sql file" >&2
    exit 1
  fi
done

echo "migration numbering check: OK"
