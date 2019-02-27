package kpatch

import (
	"encoding/base64"
	"fmt"
	"reflect"

	"github.com/ansel1/merry"
	"github.com/mikesimons/traverser"
	yaml "gopkg.in/yaml.v2"
)

type kpatch struct {
	targets        []tTarget
	missingKeyMode string
	drop           bool
	doc            map[interface{}]interface{}
	currentItem    interface{}
}

func (s *kpatch) Reset() {
	s.targets = make([]tTarget, 0)
	s.missingKeyMode = "get"
	s.drop = false
	s.doc = make(map[interface{}]interface{})
	s.currentItem = nil
}

func (s *kpatch) fnUnset(args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, merry.Errorf("unset(var, ...) requires one or more argument to unset")
	}
	for _, arg := range args {
		s.targets = append(s.targets, tTarget{opFn: traverser.Unset, target: reflect.ValueOf(arg)})
	}
	return nil, nil
}

func (s *kpatch) fnIf(args ...interface{}) (interface{}, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, merry.Errorf("if(cond, istrue, [isfalse]) takes 2 or 3 arguments")
	}

	r1 := args[1]
	var r2 interface{}

	if len(args) > 2 {
		r2 = args[2]
	}

	if args[0] == true {
		return r1, nil
	}
	return r2, nil
}

func (s *kpatch) fnNil(args ...interface{}) (interface{}, error) {
	return nil, nil
}

func (s *kpatch) fnVar(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, merry.Errorf("v(path) requires exactly one argument")
	}
	if _, ok := args[0].([]interface{}); !ok {
		return nil, merry.Errorf("v(path) expects path to be a slice")
	}
	var strArgs []string
	for _, str := range args[0].([]interface{}) {
		strArgs = append(strArgs, fmt.Sprintf("%v", str))
	}
	return traverser.GetKey(&s.doc, strArgs)
}

func (s *kpatch) fnYamlParse(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, merry.Errorf("yaml_parse(input) requires exactly one argument")
	}
	input, ok := args[0].(string)
	if !ok {
		return nil, merry.Errorf("yaml_parse(input) expects input to be a string")
	}

	var out interface{}
	var err error
	bytes, err := getInputBytes(input)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(bytes, &out)
	return out, err
}

func (s *kpatch) fnB64Decode(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, merry.Errorf("b64decode(input) requires exactly one argument")
	}
	input, ok := args[0].(string)
	if !ok {
		return nil, merry.Errorf("b64decode(input) expects input to be a string")
	}
	r, err := base64.StdEncoding.DecodeString(input)
	return string(r), err
}

func (s *kpatch) fnB64Encode(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, merry.Errorf("b64encode(input) requires exactly one argument")
	}
	input, ok := args[0].(string)
	if !ok {
		return nil, merry.Errorf("b64encode(input) expects input to be a string")
	}
	return base64.StdEncoding.EncodeToString([]byte(input)), nil
}
