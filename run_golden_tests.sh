#!/usr/bin/env bash

# This script is intended to be invoked in a github action. It will run all the
# the golden tests that it can find.

set -eEuo pipefail

# Use RUNNER_TEMP if running in a GitHub action, otherwise create a temp
# directory
if [[ -n "${RUNNER_TEMP:-}" ]]; then
  TEMP_DIR="$RUNNER_TEMP"
else
  TEMP_DIR=$(mktemp -d)
fi

# We use "go build" once instead of "go run" repeatedly because it's much
# faster.
BUILT_ABC="${TEMP_DIR}/abc-$(date +%s)"
go build -o $BUILT_ABC cmd/abc/abc.go

exit_status=0
for template_dir in $(find . -name 'testdata' | grep -v '/.git/' | xargs dirname) ; do
    echo "Running golden tests for $template_dir"
    $BUILT_ABC templates golden-test verify $template_dir
    if [[ $? != "0" ]]; then
        exit_status=1
        echo "::error title=Golden test failed::$template_dir"
fi
done

rm $BUILT_ABC

exit $exit_status
