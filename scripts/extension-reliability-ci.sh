#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist/extension-reliability-ci
exec > >(tee dist/extension-reliability-ci/checks.log) 2>&1
export CGO_ENABLED=1
go version
go test -count=1 ./extension/v1/...
go test -race -count=1 ./extension/v1/...
go build ./examples/...
go vet ./extension/v1/...
python3 - <<'PYRESULT'
import json,pathlib
pathlib.Path('dist/extension-reliability-ci/result.json').write_text(json.dumps({'status':'PASS','gates':['sdk-v1-tests','sdk-v1-race','examples-build','sdk-vet']}))
PYRESULT
