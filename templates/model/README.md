# YAML API versioning internals for developers

This is a guide for abc CLI developers on how to create a new api_version.

## When to bump the api_version

In any of the following cases:

- You're adding a new field to an existing YAML struct
- You're changing the semantics or interpretation of an existing yaml field
- You're adding a new type of YAML file

Every api_version represents a distinct configuration "language", and we never
want to change the semantics of an existing api_version that has already been
release.

## Steps

Beware, these steps may be not quite right. Please fix them or file a bug if
something isn't right.

- Announce in the abc CLI developer chat that you're bumping the api_version and
  binary version, so nobody else tries to make a conflicting simultaneous
  api_version change.
- Let "old version" mean the current api_version, and let "new version" mean the
  new api_version that you are creating. For these examples, let's suppose the
  old version is v9 and the new version is v10.
- There are a few different `kind`s of YAML files (e.g. Template, Manifest,
  GoldenTest). Each one of these has its own subdirectory under
  `templates/model`. Most api_version changes will only need to change one of
  these `kind`s. For each `kind`:
  - Locate the directory for that YAML kind (e.g. kind `Template` corresponds to
    `templates/model/spec`.
  - Create a new directory under that named after the new api_version, e.g.
    `templates/model/spec/v10`
  - Copy the contents of the previous version into your new directory (e.g.
    `cp templates/model/spec/v9/* templates/model/spec/v10/`). This includes
    `*_test.go` files.
  - Implement the `Spec.Upgrade()` method in the old schema (e.g. v9, in
    `templates/model/spec/v9/upgrade.go`) that specifies how to upgrade from the
    old schema to the new schema. See existing implementations for examples of
    how to do this simply using the
    [`copier`](https://pkg.go.dev/github.com/jinzhu/copier) library. For
    example, suppose you renamed a field, then you would implement the
    `Upgrade()` method so that it stored the contents of the old field in the
    newly renamed field.
- In `templates/model/decode/decode.go`, add a new entry to the end of
  `apiVersions`. See the instructions and examples there.
- Do a global replace of imports to point to your new version. For example, if
  you made changes to the template spec, you'd change
  `\tspec "github.com/abcxyz/abc/templates/model/spec/v9"` with
  `\tspec "github.com/abcxyz/abc/templates/model/spec/v10"`, in all Go files.
  You only need to do this for the `kind`s you changed (if you only changed the
  template spec then you don't need to do this for goldentests and manifests).
- Modify the new version directory to make whatever struct changes you want to
  make (e.g. add a new field/feature), including tests.
- Update the "list of api_versions" section in `/README.md`.
- Update the tests in decode_test.go that have "newest" in the subtest name.
  They should now use your new version instead of the formerly-newest version.
- Release a new version of the abc CLI (see `RELEASING.md`). If you've added a
  field, you only need to bump the minor version number. If you've changed the
  meaning of a field or removed a field, then you need to bump the major version
  number.
- Now you can start using your new api_version and its new features in your
  templates.

## Deprecating an api_version

TODO: write this once we do it for the first time.
