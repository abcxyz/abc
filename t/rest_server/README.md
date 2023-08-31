# Template: REST server

Template for a simple HTTP/JSON REST server implemented in Go, using go-chi for HTTP routing.

How to render this template:

1. [Install the abc binary](https://github.com/abcxyz/abc#installation).

1. cd into an empty destination directory.

    ```shell
    $ mkdir ~/rest_server
    $ cd ~/rest_server
    ```

1. See READMEs in each subfolder for more details. Render via:

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
    github.com/abcxyz/abc.git//t/rest_server/workflows
    ```

    This will output a file named `main.go` in your working directory containing the transformed program.

1. Follow the steps in the rendered README.md to run the server.
