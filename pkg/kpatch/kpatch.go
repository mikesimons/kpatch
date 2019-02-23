package kpatch

import (
	"reflect"

	"github.com/ansel1/merry"
	"github.com/mikesimons/traverser"
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
