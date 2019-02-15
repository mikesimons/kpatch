# kpatch [![Build Status](https://travis-ci.org/mikesimons/kpatch.svg?branch=master)](https://travis-ci.org/mikesimons/kpatch)

kpatch is a tool to manipulate Kubernetes manifests. It is designed to be used in build pipelines in a manner similar to standard unix tools.

kpatch was born out of a frustration with helm manifests needing tweaking and kustomizer having good manipulation capabilities but being locked behind a strange interface.

It is based around three very simple features; `selectors`, `actions` and `merges`.

## WIP :boom:
kpatch is still in early development but has been released early as it does most of what I'd intended it to. There are definitely edge cases that will cause a panic. I do not anticipate the expression language nor command line usage to change in incompatible ways but given the early days reserve the right. Code structure will definitely change.

## Selectors
Selectors use [gval]() to match manifests to process. If a manifest does not match the selector it is printed as it was read.
Some example selectors:
- `kind == "Deployment" && metadata.name == "my-nginx"`
- `metadata.label.app == "my-app"`
- `metadata.name =~ "^stdio"`

## Actions
Action expressions are manipulations that will be applied to resources. Manifests can be dropped, merged and modified.
Some example expressions:
- `drop()` - Excludes the manifest from output
- `metadata.name = metadata.name + "-my-suffix"` - Appends `-my-suffix` to `metadata.name` field.
- `metadata.labels = merge(metadata.labels, yaml("{ timestamp: 123456789 }"))`

## Merges
Merges are simple data merges of the manifest with another yaml file. Merges apply only at the root level. To merge a field use the `merge` function in an expression.

## Examples
For a more detailed set of examples, see [examples](examples)

Append a suffix to service names
```
kpatch -s 'kind == "Service"' -a 'metadata.name = metadata.name + "-service"' myyaml.yaml
```

Delete a document
```
kpatch -s 'metadata.name == "deleteme"' -a 'drop' myyaml.yaml
```

Merge common metadata in to namespace
```
cat myyaml.yaml | kpatch -s 'kind == "Namespace"' -m common-metadata.yaml
```

Apply multiple actions (to all documents)
```
kpatch -a 'name = "test"' -a 'name = name + "-test"' -a 'foo = "bar"' myyaml.yaml
```

Chain usage
```
cat myyaml.yaml | kpatch -s 'kind == "rule"' -a 'drop' | kpatch -a 'name = name + "-test"'`
```
