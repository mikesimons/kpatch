# Drop

Calling the `drop` action will cause all selected documents to be removed from output.

```
cat drop.yaml | kpatch -s 'metadata.name == "configmap2"' -a 'drop'
```

This is useful if you're dealing with a downloaded manifest where you wish to remove something.
