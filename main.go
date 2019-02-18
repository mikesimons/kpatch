package main

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"log"
	"os"
	"reflect"

	"github.com/mikesimons/kpatch/pkg/kpatch"

	"github.com/mikesimons/traverser"

	"github.com/spf13/cobra"
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
			kpatch.Run(args, selector, merges, exprs)
		},
	}

	cmd.Flags().StringVarP(&selector, "selector", "s", "", "Document selector to specify which to apply expressions / merges to.")
	cmd.Flags().StringArrayVarP(&merges, "merge", "m", merges, "YAML/JSON file or inline YAML/JSON to merge with selected documents. May be used more than once.")
	cmd.Flags().StringArrayVarP(&exprs, "action", "a", exprs, "Action expression to apply to selected documents. May be used more than once.")

	err := cmd.Execute()
	if err != nil {
		log.Fatalln("Error: ", err)
	}
}
