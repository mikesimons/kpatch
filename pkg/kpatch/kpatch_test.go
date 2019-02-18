package kpatch

import (
	"io/ioutil"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
)

func TestKpatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kpatch Suite")
}

var _ = Describe("Kpatch", func() {
	Describe("deepCopy", func() {
		It("should create a deep copy of a map[interface{}]interface{}", func() {
			test := make(map[interface{}]interface{})
			test["nested"] = make(map[interface{}]interface{})
			test["nested"].(map[interface{}]interface{})["key1"] = "val1"
			test["key2"] = "val2"

			cp, err := deepCopy(test)
			Expect(err).To(BeNil())
			Expect(cp).To(Equal(test))

			test["key2"] = "val3"

			Expect(cp).NotTo(Equal(test))
			Expect(cp["key2"]).To(Equal("val2"))
		})
	})

	Describe("inputReaderFn", func() {
		It("should return error if files aren't readable", func() {
			fn := inputReaderFn([]string{"noexisty"})
			_, err := fn()
			Expect(err).NotTo(BeNil())
		})

		It("should return an io.Reader for each file specified in args", func() {
			Fs = afero.NewMemMapFs()
			afero.WriteFile(Fs, "test.yaml", []byte("test.yaml"), 0644)

			nextInput := inputReaderFn([]string{"test.yaml"})
			contents := make([]string, 0)
			for in, err := nextInput(); in != nil && err == nil; in, err = nextInput() {
				c, _ := ioutil.ReadAll(in)
				contents = append(contents, string(c))
			}

			Expect(len(contents)).To(Equal(1))
		})

		It("should return nil when no more readers available", func() {
			nextInput := inputReaderFn([]string{})
			Expect(nextInput()).To(BeNil())
		})
	})
})
