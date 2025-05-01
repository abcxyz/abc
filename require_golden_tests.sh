#!/usr/bin/env bash

# This script is intended to be invoked in a github action. It will return a
# nonzero exit code and print a message if there are any example templates that
# don't have a golden test. This will make sure we don't break any example
# templates without realizing it.

set -eEuo pipefail

exit_status=0
# shellcheck disable=SC2044
for spec_path in $(find . -name spec.yaml); do
   template_dir="$(dirname "${spec_path}")"
   if [[ ! -d "${template_dir}/testdata/golden" ]]; then
      echo "::error title=Missing golden test for template::${template_dir}"
      exit_status=1
      fi
done
exit "${exit_status}"
