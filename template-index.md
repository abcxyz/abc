# Template index

This page contains a best-effort list of all known templates. Maybe in the
future we'll build a template repository, but for now, this page is the
"database."

## How to update

Just send a GitHub Pull Request.

## Quick installation instructions

The docs are at https://github.com/abcxyz/abc, but to recap: rendering a
template looks like this:

    abc templates render --prompt github.com/org/repo/path/to/dir@latest

... or use `--input=foo=bar` instead of `--prompt`.

## List of templates and repositories

Please keep this list sorted when adding new entries.

| Install as                                            | Docs/link                                                                                               | Description                                                                                                                            |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| github.com/abcxyz/abc/examples/templates/render/$FOO  | [link](https://github.com/abcxyz/abc/tree/main/examples/templates/render)                               | Tiny educational examples of how to use various features in your spec.yaml                                                             |
| github.com/abcxyz/abc/t/data_migration_pipeline       | [link](https://github.com/abcxyz/abc/tree/main/t/data_migration_pipeline)                               | Simple Spanner data migration pipeline in Go, using Apache Beam to migrate from a MySQL CSV dump.                                      |
| github.com/abcxyz/abc/t/react_template                | [link](https://github.com/abcxyz/abc/tree/main/t/react_template)                                        | A CRA React frontend example app intended to be extended and customized                                                                |
| github.com/abcxyz/abc/t/rest_server                   | [link](https://github.com/abcxyz/abc/tree/main/t/rest_server)                                           | A "hello world" Go HTTP server intended to be extended and customized                                                                  |
| github.com/abcxyz/gcp-org-terraform-template          | [link](https://github.com/abcxyz/gcp-org-terraform-template)                                            | Terraform files for setting up a GCP org. Restricted access.                                                                           |
| github.com/abcxyz/jvs/templates/jvs-e2e               | [link](https://github.com/abcxyz/jvs/tree/main#via-abc-cli)                                             | Terraform files for running the JVS Justification Verification Service for cryptographic justification of exceptional access.          |
| github.com/abcxyz/lumberjack/templates/lumberjack-e2e | [link](https://github.com/abcxyz/lumberjack/tree/main#via-abc-cli)                                      | Terraform files for running the Lumberjack audit logging service.                                                                      |
| github.com/abcxyz/github-token-minter/abc.templates   | [link](https://github.com/abcxyz/github-token-minter/tree/main/abc.templates#installation-with-abc-cli) | Several templates for installing and configuring the GitHub Token Minter service that helps GitHub Workflows elevate their privileges. |
