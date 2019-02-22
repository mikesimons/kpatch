package kpatch

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"github.com/ansel1/merry"

	"github.com/mikesimons/traverser"

	"github.com/PaesslerAG/gval"
	"github.com/imdario/mergo"
	yaml "gopkg.in/yaml.v2"
)

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
	if err != nil {
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

func Run(args []string, selector string, merges []string, exprs []string, output io.WriteCloser) error {
	var err error
	var input io.Reader
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
			return merry.Wrap(err)
		}

		// State
		var targets []tTarget
		var sets []tSet
		missingKeyMode := "get"
		drop := false
		thing := make(map[interface{}]interface{})

		lang := gval.NewLanguage(gval.Full(),
			gval.VariableSelector(func(path gval.Evaluables) gval.Evaluable {
				return func(c context.Context, v interface{}) (interface{}, error) {
					keys, _ := path.EvalStrings(c, v)

					if keys[0] == "__root" {
						if len(keys) == 1 {
							return thing, nil
						}
						keys = keys[1:]
					}

					val, err := traverser.GetKey(&thing, keys)

					if err != nil && missingKeyMode == "set" {
						err := traverser.SetKey(&thing, keys, "")
						if err != nil {
							return nil, err
						}
						return traverser.GetKey(&thing, keys)
					}

					return val, err
				}
			}),
			gval.Function("parse_yaml", func(args ...interface{}) (interface{}, error) {
				var out interface{}
				var err error
				bytes, err := getInputBytes(args[0].(string))
				if err != nil {
					return nil, err
				}
				err = yaml.Unmarshal(bytes, &out)
				return out, err
			}),
			gval.Function("dump_yaml", func(args ...interface{}) (interface{}, error) {
				r, err := yaml.Marshal(args[0])
				return string(r), err
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
			gval.Function("var", func(args ...interface{}) (interface{}, error) {
				strArgs := make([]string, 0)
				for _, a := range args {
					strArgs = append(strArgs, a.(string))
				}
				return traverser.GetKey(&thing, strArgs)
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
				r, err := base64.StdEncoding.DecodeString(args[0].(string))
				return string(r), err
			}),
			gval.Function("b64encode", func(args ...interface{}) (interface{}, error) {
				return base64.StdEncoding.EncodeToString([]byte(args[0].(string))), nil
			}),
			gval.InfixEvalOperator("=", func(a, b gval.Evaluable) (gval.Evaluable, error) {
				if !b.IsConst() {
					return func(c context.Context, o interface{}) (interface{}, error) {
						missingKeyMode = "set"
						target, err := a(c, o)
						if err != nil {
							return nil, err
						}

						missingKeyMode = "get"
						val, err := b(c, o)

						if err != nil {
							return nil, err
						}

						if reflect.ValueOf(target) == reflect.ValueOf(thing) {
							thing = val.(map[interface{}]interface{})
							return nil, nil
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
				val, err := b(nil, nil)
				if err != nil {
					return nil, err
				}

				return func(c context.Context, v interface{}) (interface{}, error) {
					missingKeyMode = "set"
					target, err := a(c, v)
					if err != nil {
						return nil, err
					}

					if reflect.ValueOf(target) == reflect.ValueOf(thing) {
						thing = val.(map[interface{}]interface{})
						return nil, nil
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

		for decoder.Decode(&thing) == nil {
			if len(thing) == 0 {
				continue
			}

			var value interface{}
			if selector != "" {
				value, err = gval.Evaluate(selector, thing)
				if err != nil {
					return merry.Wrap(err).WithUserMessagef("error evaluating selector: %s", err)
				}
			} else {
				value = interface{}(true)
			}

			if value == true {
				if len(merges) > 0 {
					for _, m := range mergeData {
						mCopy, err := deepCopy(m)
						if err != nil {
							return merry.Wrap(err).WithUserMessagef("error copying merge data: %s", err)
						}

						err = mergo.Map(&thing, mCopy, mergo.WithOverride)
						if err != nil {
							return merry.Wrap(err).WithUserMessagef("error merging: %s", err)
						}
					}
				}

				if len(exprs) > 0 {
					for _, expr := range exprs {
						targets = make([]tTarget, 0)
						sets = make([]tSet, 0)

						_, err := lang.Evaluate(expr, thing)
						if err != nil {
							return merry.Wrap(err).WithUserMessagef("error evaluating action expression '%s': %s", expr, err)
						}

						for _, s := range sets {
							if err = traverser.SetKey(&thing, strings.Split(s.key, "."), s.value); err != nil {
								return merry.Wrap(err).WithUserMessagef("could not set key '%s': %s", s.key, err)
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
							return merry.Wrap(err).WithUserMessagef("error applying changes: %s", err)
						}
					}
				}
			}

			if !drop {
				if err = encoder.Encode(thing); err != nil {
					return merry.Wrap(err).WithUserMessagef("error encoding output: %s", err)
				}
			}

			// Reset state
			drop = false
			thing = make(map[interface{}]interface{})
		}
	}

	if err != nil {
		return merry.Wrap(err).WithUserMessagef("unknown error: %s", err)
	}

	return nil
}
