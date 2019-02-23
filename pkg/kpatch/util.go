package kpatch

import (
	"bytes"
	"encoding/gob"
	"io"
	"os"

	"github.com/spf13/afero"
)

var Fs = afero.NewOsFs()

func init() {
	gob.Register(map[interface{}]interface{}{})
	gob.Register([]interface{}{})
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

func inputReaderFn(inputs []string) func() (io.Reader, error) {
	current := 0
	return func() (io.Reader, error) {
		if current >= len(inputs) {
			return nil, nil
		}

		input := inputs[current]
		current++

		if input == "-" {
			return io.Reader(os.Stdin), nil
		}

		_, err := Fs.Stat(input)
		if os.IsNotExist(err) {
			return nil, err
		}

		return Fs.Open(input)
	}
}
