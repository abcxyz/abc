# Template: REST server

Template for a simple HTTP/JSON REST server implemented in Go, using go-chi for HTTP routing.

How to render this template:

1. [Install the abc binary](https://github.com/abcxyz/abc#installation).

1. Render via:

    ```shell
    $ abc templates render github.com/abcxyz/abc.git//t/rest_server/code

    $ abc templates render github.com/abcxyz/abc.git//t/rest_server/deployments

    $ abc templates render --input="automation_service_account=[automation_service_account]" \
    --input="wif_provider=[wif_provider]" \
    --input="ar_repository=[ar_repository]" \
    --input="ar_location=[ar_location]" \
    --input="cr_service=[cr_service]" \
    --input="region=[region]" \
    --input="project_id=[project_id]" \
    --input="code_subfolder=[code_subfolder]" \
    --input="deployments_subfolder=[deployments_subfolder]" \
    github.com/abcxyz/abc.git//t/rest_server/workflows
    ```

1. Follow the steps in the rendered README.md to run the server.

1. Optionally, render the CI/CD workflows as well (see subfolder README for more details).
