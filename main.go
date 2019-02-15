package main

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"io"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/mikesimons/traverser"

	"github.com/PaesslerAG/gval"
	"github.com/PaesslerAG/jsonpath"
	"github.com/imdario/mergo"
	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v2"
)

var versionString = "dev"

type tTarget struct {
	opFn   func() (traverser.Op, error)
	target reflect.Value
}

type tSet struct {
	key   string
	value interface{}
}

func deepCopy(m map[interface{}]interface{}) (map[interface{}]interface{}, error) {
	var buf bytes.Buffer
	var out map[interface{}]interface{}

	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)

	errs := []error{enc.Encode(m), dec.Decode(&out)}

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

func getInputBytes(input string) ([]byte, error) {
	_, err := os.Stat(input)
	if os.IsNotExist(err) {
		return []byte(input), nil
	}

	f, err := os.Open(input)
	if err != nil {
		return []byte{}, err
	}
	defer f.Close()

	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		return []byte{}, err
	}
	return bytes, nil
}

func main() {
	var selector string
	var merges []string
	var exprs []string

	cmd := &cobra.Command{
		Use:     "kpatch",
		Version: versionString,
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			var input io.Reader
			output := os.Stdout
			defer output.Close()
			encoder := yaml.NewEncoder(output)

			if len(args) == 0 {
				input = os.Stdin
			} else {
				input, err = os.Open(args[0])
				if err != nil {
					log.Fatalln("Error opening input: ", err)
				}
			}

			decoder := yaml.NewDecoder(input)
			gob.Register(map[interface{}]interface{}{})

			var mergeData []map[interface{}]interface{}
			if len(merges) > 0 {
				for _, m := range merges {
					merge := make(map[interface{}]interface{})
					mergeBytes, err := getInputBytes(m)
					if err != nil {
						log.Fatalf("Error loading merge '%s': %s\n", m, err)
					}

					err = yaml.Unmarshal(mergeBytes, &merge)
					if err != nil {
						log.Fatalf("Error parsing merge '%s': %s\n", m, err)
					}
					mergeData = append(mergeData, merge)
				}
			}

			// State
			var targets []tTarget
			var sets []tSet
			drop := false

			lang := gval.NewLanguage(gval.Full(),
				jsonpath.Language(),
				gval.Function("yaml", func(args ...interface{}) (interface{}, error) {
					var out interface{}
					var err error
					bytes, err := getInputBytes(args[0].(string))
					if err != nil {
						return nil, err
					}
					err = yaml.Unmarshal(bytes, &out)
					return out, err
				}),
				gval.Function("merge", func(args ...interface{}) (interface{}, error) {
					var err error
					out := make(map[interface{}]interface{})
					a := args[0].(map[interface{}]interface{})
					b := args[1].(map[interface{}]interface{})

					err = mergo.Map(&out, a)
					if err != nil {
						return nil, err
					}

					err = mergo.Map(&out, b, mergo.WithOverride)
					if err != nil {
						return nil, err
					}

					return out, nil
				}),
				gval.Function("set", func(args ...interface{}) (interface{}, error) {
					sets = append(sets, tSet{key: args[0].(string), value: args[1]})
					return nil, nil
				}),
				gval.Function("unset", func(args ...interface{}) (interface{}, error) {
					targets = append(targets, tTarget{opFn: traverser.Unset, target: reflect.ValueOf(args[0])})
					return nil, nil
				}),
				gval.Function("drop", func(args ...interface{}) (interface{}, error) {
					drop = true
					return nil, nil
				}),
				gval.Function("b64decode", func(args ...interface{}) (interface{}, error) {
					return base64.StdEncoding.DecodeString(args[0].(string))
				}),
				gval.Function("b64encode", func(args ...interface{}) (interface{}, error) {
					return base64.StdEncoding.EncodeToString([]byte(args[0].(string))), nil
				}),
				gval.InfixOperator("=", func(a, b interface{}) (interface{}, error) {
					targets = append(
						targets,
						tTarget{
							opFn: func() (traverser.Op, error) {
								return traverser.Set(reflect.ValueOf(b))
							},
							target: reflect.ValueOf(a),
						},
					)
					return b, nil
				}),
			)

			thing := make(map[interface{}]interface{})
			for decoder.Decode(&thing) == nil {
				if len(thing) == 0 {
					continue
				}

				var value interface{}
				if selector != "" {
					value, err = gval.Evaluate(selector, thing)
					if err != nil {
						log.Fatalln("Error evaluating selector: ", err)
					}
				} else {
					value = interface{}(true)
				}

				if value == true {
					if len(merges) > 0 {
						for _, m := range mergeData {
							mCopy, err := deepCopy(m)
							if err != nil {
								log.Fatalln("Internal error copying merge data: ", err)
							}

							err = mergo.Map(&thing, mCopy, mergo.WithOverride)
							if err != nil {
								log.Fatalln("Error merging merge: ", err)
							}
						}
					}

					if len(exprs) > 0 {
						for _, expr := range exprs {
							targets = make([]tTarget, 0)
							sets = make([]tSet, 0)

							_, err := lang.Evaluate(expr, thing)
							if err != nil {
								log.Fatalf("Error evaluating 'set' expression '%s': %s\n", expr, err)
							}

							for _, s := range sets {
								if err = traverser.SetKey(&thing, strings.Split(s.key, "."), s.value); err != nil {
									log.Fatalf("Could not set key '%s': %s", s.key, err)
								}
							}

							t := &traverser.Traverser{
								Node: func(keys []string, data reflect.Value) (traverser.Op, error) {
									for _, target := range targets {
										if data == target.target {
											return target.opFn()
										}
									}
									return traverser.Noop()
								},
							}

							if err = t.Traverse(reflect.ValueOf(thing)); err != nil {
								log.Fatalln("Error applying changes: ", err)
							}
						}
					}
				}

				if !drop {
					if err = encoder.Encode(thing); err != nil {
						log.Fatalln("Error encoding output:", err)
					}
				}

				// Reset state
				drop = false
				thing = make(map[interface{}]interface{})
			}
		},
	}

	cmd.Flags().StringVarP(&selector, "selector", "s", "", "Document selector to specify which to apply expressions / merges to.")
	cmd.Flags().StringArrayVarP(&merges, "merge", "m", merges, "YAML/JSON file or inline YAML/JSON to merge with selected documents. May be used more than once.")
	cmd.Flags().StringArrayVarP(&exprs, "expr", "e", exprs, "Expression to apply to selected documents. May be used more than once.")

	err := cmd.Execute()
	if err != nil {
		log.Fatalln("Error: ", err)
	}
}
