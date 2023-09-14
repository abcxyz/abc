# Template index

This page contains a best-effort list of all known templates. Maybe in the
future we'll build a template repository, but for now, this page is the
"database."

## How to update

Just send a GitHub Pull Request.

## Quick installation instructions

The docs are at https://github.com/abcxyz/abc, but to recap: rendering a
template looks like this:

    abc templates render --prompt github.com/org/repo.git//path/to/dir

... or use `--input=foo=bar` instead of `--prompt`.

## List of templates and repositories

Please keep this list sorted when adding new entries.

| Install as                                                | Docs/link                                                                 | Description                                                                                       |
| --------------------------------------------------------- | ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| github.com/abcxyz/abc.git//examples/templates/render/$FOO | [link](https://github.com/abcxyz/abc/tree/main/examples/templates/render) | Tiny educational examples of how to use various features in your spec.yaml                        |
| github.com/abcxyz/abc.git//t/data_migration_pipeline      | [link](https://github.com/abcxyz/abc/tree/main/t/data_migration_pipeline) | Simple Spanner data migration pipeline in Go, using Apache Beam to migrate from a MySQL CSV dump. |
| github.com/abcxyz/abc.git//t/react_template               | [link](https://github.com/abcxyz/abc/tree/main/t/react_template)          | A CRA React frontend example app intended to be extended and customized                               |
| github.com/abcxyz/abc.git//t/rest_server                  | [link](https://github.com/abcxyz/abc/tree/main/t/rest_server)             | A "hello world" Go HTTP server intended to be extended and customized                             |
| github.com/abcxyz/gcp-org-terraform-template.git          | [link](https://github.com/abcxyz/gcp-org-terraform-template)              | Terraform files for setting up a GCP org. Restricted access.                                                          |
