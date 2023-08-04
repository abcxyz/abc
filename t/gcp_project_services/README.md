# Example: GCP Project Services

Enables GCP services for a specific project in an environment specified by the [gcp-org-terraform-template](https://github.com/abcxyz/gcp-org-terraform-template).

1. [Install the `abc` binary](https://github.com/abcxyz/abc/blob/main/README.md#installation).

1. Render this template, this will output several Terraform files that can be applied to a GCP project. See spec.yaml for more details.

1. Terraform init and apply the project changes.

    ```shell
    terraform init -backend=false
    ```
