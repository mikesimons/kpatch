# kpatch

kpatch is a tool to manipulate Kubernetes manifests. It is designed to be used in build pipelines in a manner similar to standard unix tools.

kpatch was born out of a frustration with helm manifests needing tweaking and kustomizer having good manipulation capabilities but being locked behind a strange interface.

It is based around three very simple features; `selectors`, `expressions` and `merges`.

## Selectors
Selectors use [gval]() to match manifests to process. If a manifest does not match the selector it is printed as it was read.
Some example selectors:
- `kind == "Deployment" && metadata.name == "my-nginx"`
- `metadata.label.app == "my-app"`
- `metadata.name =~ "^stdio"`

## Expressions
Expressions are manipulations that will be applied to resources. Manifests can be dropped, merged and modified.
Some example expressions:
- `drop()` - Excludes the manifest from output
- `metadata.name = metadata.name + "-my-suffix"` - Appends `-my-suffix` to `metadata.name` field.
- `metadata.labels = merge(metadata.labels, yaml("{ timestamp: 123456789 }"))`

## Merges
Merges are simple data merges of the manifest with another yaml file. Merges apply only at the root level. To merge a field use the `merge` function in an expression.

## Examples
See [examples](examples)
