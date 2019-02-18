package kpatch

import (
	"context"
	"encoding/base64"
	"fmt"
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

func getMergeData(merges []string) ([]map[interface{}]interface{}, error) {
	var mergeData []map[interface{}]interface{}
	if len(merges) > 0 {
		for _, m := range merges {
			merge := make(map[interface{}]interface{})
			mergeBytes, err := getInputBytes(m)
			if err != nil {
				return nil, fmt.Errorf("error loading merge '%s': %s", m, err)
			}

			err = yaml.Unmarshal(mergeBytes, &merge)
			if err != nil {
				return nil, fmt.Errorf("error parsing merge '%s': %s", m, err)
			}
			mergeData = append(mergeData, merge)
		}
	}
	return mergeData, nil
}

func Run(args []string, selector string, merges []string, exprs []string) {
	var err error
	var input io.Reader
	output := os.Stdout
	defer output.Close()
	encoder := yaml.NewEncoder(output)

	if len(args) == 0 {
		args = []string{"-"}
	}

	nextInput := inputReaderFn(args)

	for input, err = nextInput(); input != nil && err == nil; input, err = nextInput() {

		decoder := yaml.NewDecoder(input)

		mergeData, err := getMergeData(merges)
		if err != nil {
			log.Fatal(err)
		}

		// State
		var targets []tTarget
		var sets []tSet
		missingKeyMode := "get"
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
			gval.InfixEvalOperator("=", func(a, b gval.Evaluable) (gval.Evaluable, error) {
				if !b.IsConst() {
					return func(c context.Context, o interface{}) (interface{}, error) {
						missingKeyMode = "set"
						target, err := a.EvalInterface(c, o)
						if err != nil {
							return nil, err
						}

						missingKeyMode = "get"
						val, err := b.EvalInterface(c, o)

						if err != nil {
							return nil, err
						}

						targets = append(
							targets,
							tTarget{
								opFn: func() (traverser.Op, error) {
									return traverser.Set(reflect.ValueOf(val))
								},
								target: reflect.ValueOf(interface{}(target)),
							},
						)
						return nil, nil
					}, nil
				}
				val, err := b.EvalInterface(nil, nil)
				if err != nil {
					return nil, err
				}

				return func(c context.Context, v interface{}) (interface{}, error) {
					missingKeyMode = "set"
					target, err := a.EvalInterface(c, v)
					if err != nil {
						return nil, err
					}

					missingKeyMode = "get"
					targets = append(
						targets,
						tTarget{
							opFn: func() (traverser.Op, error) {
								return traverser.Set(reflect.ValueOf(val))
							},
							target: reflect.ValueOf(target),
						},
					)
					return nil, nil
				}, nil
			}),
		)

		thing := make(map[interface{}]interface{})
		for decoder.Decode(&thing) == nil {
			if len(thing) == 0 {
				continue
			}

			lang.MissingVarHandler(func(keys []string, index int) (interface{}, error) {
				if missingKeyMode == "set" {
					err := traverser.SetKey(&thing, keys, "")
					if err != nil {
						return nil, err
					}
					return traverser.GetKey(&thing, keys)
				} else {
					if index == len(keys)-1 {
						return "", nil
					}
					return nil, fmt.Errorf("unknown parameter %s", strings.Join(keys[:index+1], "."))
				}
			})

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
							log.Fatalf("Error evaluating action expression '%s': %s\n", expr, err)
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
	}

	if err != nil {
		log.Fatalln(err)
	}
}
