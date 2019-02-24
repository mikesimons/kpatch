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

		kp := &kpatch{
			missingKeyMode: "get",
			doc:            make(map[interface{}]interface{}),
		}

		lang := gval.NewLanguage(gval.Full(),
			gval.PostfixOperator("|", func(c context.Context, p *gval.Parser, e gval.Evaluable) (gval.Evaluable, error) {
				// a = after operator
				a, err := p.ParseExpression(c)
				if err != nil {
					return nil, err
				}

				return func(c context.Context, v interface{}) (interface{}, error) {

					// x = before operator
					x, err := e(c, v)
					if err != nil {
						return nil, err
					}

					// Make input a slice if it's not already
					if reflect.ValueOf(x).Kind() != reflect.Slice {
						x = []interface{}{x}
					}

					// Apply RHS for every element of LHS
					var out []interface{}
					for _, item := range x.([]interface{}) {
						tmp := kp.currentItem
						kp.currentItem = item
						z, err := a(c, v)
						if err != nil {
							log.Fatal("pipeline error: ", err)
						}
						if z != nil {
							out = append(out, z)
						}
						kp.currentItem = tmp
					}

					return out, nil
				}, nil
			}),
			gval.VariableSelector(func(path gval.Evaluables) gval.Evaluable {
				return func(c context.Context, v interface{}) (interface{}, error) {
					var root interface{}
					keys, _ := path.EvalStrings(c, v)
					root = kp.doc

					if keys[0] == "@" {
						root = kp.currentItem
						keys = keys[1:]
					}

					if len(keys) == 0 {
						return root, nil
					}

					val, err := traverser.GetKey(&root, keys)

					if err != nil && kp.missingKeyMode == "set" {
						err := traverser.SetKey(&root, keys, "")
						if err != nil {
							return nil, err
						}
						return traverser.GetKey(&root, keys)
					}

					err = nil

					return val, err
				}
			}),
			gval.Function("print", func(args ...interface{}) (interface{}, error) {
				fmt.Print(args...)
				return nil, nil
			}),
			gval.Function("if", kp.fnIf),
			gval.Function("nil", kp.fnNil),
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
			gval.Function("v", func(args ...interface{}) (interface{}, error) {
				if len(args) == 1 {
					args = append(args, "/")
				}
				strArgs := strings.Split(args[0].(string), args[1].(string))
				return traverser.GetKey(&kp.doc, strArgs)
			}),
			gval.Function("unset", kp.fnUnset),
			gval.Function("drop", func(args ...interface{}) (interface{}, error) {
				kp.drop = true
				return nil, nil
			}),
			gval.Function("b64decode", func(args ...interface{}) (interface{}, error) {
				r, err := base64.StdEncoding.DecodeString(args[0].(string))
				return string(r), err
			}),
			gval.Function("b64encode", func(args ...interface{}) (interface{}, error) {
				return base64.StdEncoding.EncodeToString([]byte(args[0].(string))), nil
			}),
			gval.Function("B64ENCODE", func(args ...interface{}) (interface{}, error) {
				val := base64.StdEncoding.EncodeToString([]byte(args[0].(string)))
				kp.targets = append(kp.targets, tTarget{opFn: func() (traverser.Op, error) { return traverser.Set(reflect.ValueOf(val)) }, target: reflect.ValueOf(args[0])})
				return nil, nil
			}),
			gval.InfixEvalOperator("=", func(a, b gval.Evaluable) (gval.Evaluable, error) {
				if !b.IsConst() {
					return func(c context.Context, o interface{}) (interface{}, error) {
						kp.missingKeyMode = "get"
						val, err := b(c, o)

						if err != nil {
							return nil, err
						}

						if reflect.ValueOf(val) == reflect.ValueOf(kp.doc) {
							val, err = deepCopy(kp.doc)
							if err != nil {
								log.Fatalln("Couldn't deep copy root: ", err)
							}
						}

						kp.missingKeyMode = "set"
						target, err := a(c, o)
						if err != nil {
							return nil, err
						}

						kp.missingKeyMode = "get"

						if reflect.ValueOf(target) == reflect.ValueOf(kp.doc) {
							kp.doc = val.(map[interface{}]interface{})
							return nil, nil
						}

						kp.targets = append(
							kp.targets,
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
					kp.missingKeyMode = "set"
					target, err := a(c, v)
					if err != nil {
						return nil, err
					}

					if reflect.ValueOf(target) == reflect.ValueOf(kp.doc) {
						kp.doc = val.(map[interface{}]interface{})
						return nil, nil
					}

					kp.missingKeyMode = "get"
					kp.targets = append(
						kp.targets,
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

		for decoder.Decode(&kp.doc) == nil {
			if len(kp.doc) == 0 {
				continue
			}

			kp.currentItem = kp.doc

			var value interface{}
			if selector != "" {
				value, err = gval.Evaluate(selector, kp.doc)
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

						err = mergo.Map(&kp.doc, mCopy, mergo.WithOverride)
						if err != nil {
							return merry.Wrap(err).WithUserMessagef("error merging: %s", err)
						}
					}
				}

				if len(exprs) > 0 {
					for _, expr := range exprs {
						kp.targets = make([]tTarget, 0)
						//sets = make([]tSet, 0)

						_, err := lang.Evaluate(expr, kp.doc)
						if err != nil {
							return merry.Wrap(err).WithUserMessagef("action expression error: %s", err)
						}

						/*for _, s := range sets {
							if err = traverser.SetKey(&thing, strings.Split(s.key, "."), s.value); err != nil {
								return merry.Wrap(err).WithUserMessagef("could not set key '%s': %s", s.key, err)
							}
						}*/

						t := &traverser.Traverser{
							Node: func(keys []string, data reflect.Value) (traverser.Op, error) {
								for _, target := range kp.targets {
									if data == target.target {
										return target.opFn()
									}
								}
								return traverser.Noop()
							},
						}

						if err = t.Traverse(reflect.ValueOf(kp.doc)); err != nil {
							return merry.Wrap(err).WithUserMessagef("error applying changes: %s", err)
						}
					}
				}
			}

			if !kp.drop {
				if err = encoder.Encode(kp.doc); err != nil {
					return merry.Wrap(err).WithUserMessagef("error encoding output: %s", err)
				}
			}

			kp.Reset()
		}
	}

	if err != nil {
		return merry.Wrap(err).WithUserMessagef("unknown error: %s", err)
	}

	return nil
}
