# REST server workflows template

This directory contains the CI/CD GitHub Action workflows for the REST server template.

    $ abc templates render -input="automation_service_account=[automation_service_account]" \
    -input="wif_provider=[wif_provider]" \
    -input="ar_repository=[ar_repository]" \
    -input="ar_location=[ar_location]" \
    -input="cr_service=[cr_service]" \
    -input="region=[region]" \
    -input="project_id=[project_id]" \
    github.com/abcxyz/abc.git//t/rest_server/workflows
