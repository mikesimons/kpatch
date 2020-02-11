# mutating vs non-mutating
Two variants of FNs that operate on a variable:
- b64enc(input) - returns input base64 encoded
- B64ENC(input) - sets input to base64 encoded value

```
-a 'B64DEC(var("data", "filebeat.yml"))'
-a 'UNSET(var("data","filebeat.yml","output.file"))'
-a 'var("data", "filebeat.yml", "output.redis", "hosts") = ["test"]'
-a 'B64ENC(var("data", "filebeat.yml"))'

-a 'B64DEC(v("data/filebeat.yml"))
-a 'UNSET(v("data/filebeat.yml/output.file"))'
-a 'v("data/filebeat.yml/output.redis") = ["test"]'
-a 'B64ENC(v("data/filebeat.yml"))
```

# helpers
Loosely based on sprig...
- b64enc(input) -> base64 encoded input
- b64dec(input) -> base64 decoded input
- parse_yaml(input) -> Unmarshalled yaml
- dump_yaml(input) -> Marshalled yaml
- merge(input1, input2) -> Merge input2 in to input1
- unset([arg]) -> unsets arg or last root if not provided
- concat(arg, arg, arg, ...) -> concatenates arguments. Supports arrays + strings
- prefix(input, prefix) -> prefixed input (if not already prefixed)
- suffix(input, suffix) -> suffixed input (if not already suffixed)
- split(input, sep) -> Split string on sep
- join(input, sep) -> Join string with sep

- dump_json(input) -> Marshalled json
- unixtime -> Unixtime UTC
- sha256

- exec("someprocess", var) - If var is string is piped to process. If is map, is yaml encoded.
- list | if(match, @) | @ = [@, "moo", "baz"] # Where @ is array, replace the array otherwise merge with parent?
- list | if(match, @) | @ = ["moo", "baz", @] # Where @ is array, replace the array oterhwise merge with parent?

# Roadmap
- Support for interpolation in input vars after all other processing
  - Everything between delimeters parsed as selector gval: {{ @params.cpu & @params.cpu_burst_factor }}m
  - Delimeters configurable through : --delim "{{,}}"
- Add tests for all fns & move them to over to kpatch type
- Provide mutating fns where they make sense
- Add @param support for selector
- Make @param error if written to ?
- Replicate datadog chart with kpatch for giggles
- Replicate istio chart with kpatch for giggles


```
kgen deployment | \
kpatch \
  -a 'c = yaml_parse("container.yaml")' \
  -a 'c.resources.limits = { cpu: concat(@params.cpu * @params.cpu_burst_factor, "m"), memory: @params.memory }' \
  -a 'c.resources.requests = { cpu: @params.cpu, memory: @params.memory }' \
  -a 'spec.template.metadata.labels.deploy = @params.deploy_label' \
  -a 'spec.selector.matchLabels.deploy = @params.deploy_label' \
  -a 'spec.template.replicas = @params.replicas' \
  -a 'spec.template.spec.containers = [load("partials/container.yaml"), load("partials/prometheus-exporter-sidecar.yaml")]'
```