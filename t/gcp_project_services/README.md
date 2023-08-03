# Example: GCP Project Services

Enables GCP services for a specific project in an environment specified by the [gcp-org-terraform-template](https://github.com/abcxyz/gcp-org-terraform-template).

1. cd into the resources/[product_id] directory

    ```shell
    $ cd gcp-org-terraform-template/resources/[product_id]
    ```

1. Install the `abc` binary

    ```shell
    $ go install github.com/abcxyz/abc/cmd/abc@latest
    $ abc --help
    ```

    This only works if you have go installed (https://go.dev/doc/install) and have the Go binary directory in your $PATH (try PATH=$PATH:~/go/bin).

1. Execute the template defined in the `t` directory.
This will output several Terraform files that can be applied to a project.

    ```shell
    $ abc templates render \
    -input="product_id=abc-demo" \
    -input="project_id=abc-demo-a-6a5eab" \
    -input="services=artifactregistry.googleapis.com,iam.googleapis.com,run.googleapis.com" \
    -input="environment=autopush" \
    -input="bucket_name=abcxyz-tycho-terraform-state-f916" \
    -input="bucket_prefix=infra" \
    -dest="./test" \
    -spec="spec.yaml" \
    ~/gitrepos/abc/t/gcp_project_services
    ```

1. Terraform init and apply the project changes.

    ```shell
    terraform init -backend=false
    ```