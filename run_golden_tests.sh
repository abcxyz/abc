#!/usr/bin/env bash

# This script is intended to be invoked in a github action. It will run all the
# the golden tests that it can find.

set -eEuo pipefail

exit_status=0
for template_dir in $(find . -name 'testdata' | grep -v '/.git/' | xargs dirname) ; do
    echo "Running golden tests for $template_dir"
    go run cmd/abc/abc.go templates golden-test verify $template_dir
    if [[ $? != "0" ]]; then
        exit_status=1
        echo "::error title=Golden test failed::$template_dir"
fi
done
exit $exit_status
