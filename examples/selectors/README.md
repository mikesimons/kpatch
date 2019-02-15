# Selectors

Selectors and action expressions in kpatch use [gval](https://github.com/PaesslerAG/gval).
The selector will be evaluated against each document in the input stream.

Since gval is a pretty full featured expression language covering it all is not possible but listed below are some things to give you an idea.

We're using the (drop)[../drop] function to make it clear which documents are being (because they go missing).

## Field match
```
kpatch -s 'type == "thing"' -a 'drop' selectors.yaml
```
Output:
```
name: cat in a hat
type: cat
```

## Negated field match
```
kpatch -s 'name != "thing1"' -a 'drop' selectors.yaml
```
Output:
```
name: thing1
some:
  data: data
type: thing
```

## Regex
```
kpatch -s '!(name =~ ".*3$")' -a 'drop' selectors.yaml
```
Output:
```
name: thing3
some:
- otherdata
type: thing
```

## Combining with boolean expressions
```
kpatch -s 'name == "thing1 || name == "thing2"' -a 'drop' selectors.yaml
```
Output:
```
name: thing3
some:
- otherdata
type: thing
---
name: cat in a hat
type: cat
```
