package kpatch

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ansel1/merry"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
	yaml "gopkg.in/yaml.v2"
)

func TestKpatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kpatch Suite")
}

func dorun(fn func(f io.WriteCloser)) []byte {
	fs := afero.NewMemMapFs()
	f, _ := fs.OpenFile("output.yaml", os.O_CREATE|os.O_WRONLY, os.ModeAppend)

	fn(f)
	f.Close()

	f, _ = fs.Open("output.yaml")
	defer f.Close()

	data, _ := ioutil.ReadAll(f)

	return data
}

func decodeDocs(input []byte) []map[interface{}]interface{} {
	decoder := yaml.NewDecoder(bytes.NewReader(input))
	var docs []map[interface{}]interface{}
	for {
		doc := make(map[interface{}]interface{})
		err := decoder.Decode(&doc)
		if err != nil {
			break
		}

		docs = append(docs, doc)
	}
	return docs
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
			defer func() { Fs = afero.NewOsFs() }()
			_ = afero.WriteFile(Fs, "test.yaml", []byte("test.yaml"), 0644)

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

	Describe("Run", func() {
		It("should process multiple inputs with multiple documents in each", func() {
			data := dorun(func(f io.WriteCloser) {
				Run([]string{"testdata/input1.yaml", "testdata/input2.yaml"}, "", []string{}, []string{}, f)
			})

			docs := decodeDocs(data)
			Expect(docs).To(HaveLen(4))
		})

		It("should apply all actions to documents that match the selector", func() {
			data := dorun(func(f io.WriteCloser) {
				Run([]string{"testdata/input1.yaml", "testdata/input2.yaml"}, "name =~ \".*document2\"", []string{}, []string{"test1 = 1", "test2 = 2"}, f)
			})

			docs := decodeDocs(data)
			Expect(docs).To(HaveLen(4))
			for i, doc := range docs {
				if i%2 == 0 {
					Expect(doc["test1"]).To(BeNil())
					Expect(doc["test2"]).To(BeNil())
				} else {
					Expect(doc["test1"]).To(Equal(1))
					Expect(doc["test2"]).To(Equal(2))
				}
			}
		})

		It("should print documents without processing that do not match the selector", func() {
			data := dorun(func(f io.WriteCloser) {
				Run([]string{"testdata/input1.yaml", "testdata/input2.yaml"}, "false", []string{}, []string{}, f)
			})

			docs := decodeDocs(data)
			Expect(docs).To(HaveLen(4))
		})

		Describe("selector", func() {
			It("should only match documents that match expression", func() {
				data := dorun(func(f io.WriteCloser) {
					Run([]string{"testdata/input1.yaml", "testdata/input2.yaml"}, "name =~ \".*document1\"", []string{}, []string{"drop"}, f)
				})

				docs := decodeDocs(data)

				Expect(docs).To(HaveLen(2))
				Expect(docs[0]["name"]).To(Equal("input1document2"))
				Expect(docs[1]["name"]).To(Equal("input2document2"))
			})

			It("should not allow assignments in selector", func() {
				var e error
				dorun(func(f io.WriteCloser) {
					e = Run([]string{"testdata/input1.yaml"}, "test = 123", []string{}, []string{}, f)
				})

				Expect(e).NotTo(BeNil())
				Expect(merry.UserMessage(e)).To(ContainSubstring("unexpected \"=\""))
			})

			// TODO: Expand to other mutating functions (e.g. merge)
			It("should not allow mutating functions in selector", func() {
				var e error
				dorun(func(f io.WriteCloser) {
					e = Run([]string{"testdata/input1.yaml"}, "unset(name)", []string{}, []string{}, f)
				})

				Expect(e).NotTo(BeNil())
				Expect(merry.UserMessage(e)).To(ContainSubstring("could not call 'unset'"))
			})

			It("should match all documents if not specified", func() {
				data := dorun(func(f io.WriteCloser) {
					Run([]string{"testdata/input1.yaml", "testdata/input2.yaml"}, "", []string{}, []string{"drop"}, f)
				})

				docs := decodeDocs(data)

				Expect(docs).To(HaveLen(0))
			})
		})

		Describe("Action language", func() {
			Describe("drop", func() {
				It("should drop matching documents", func() {
					data := dorun(func(f io.WriteCloser) {
						Run([]string{"testdata/input1.yaml"}, "name == \"input1document1\"", []string{}, []string{"drop"}, f)
					})

					docs := decodeDocs(data)
					names := make([]string, 0)
					for _, doc := range docs {
						names = append(names, doc["name"].(string))
					}

					Expect(names).To(HaveLen(1))
					Expect(names).NotTo(ContainElement("input1document1"))
				})
			})

			Describe("assign", func() {
				It("should set field if action is assignment", func() {
					data := dorun(func(f io.WriteCloser) {
						Run([]string{"testdata/input1.yaml"}, "", []string{}, []string{"maptype = \"hello\""}, f)
					})

					docs := decodeDocs(data)

					Expect(docs[0]["maptype"]).To(Equal("hello"))
				})

				It("should create key if key on left hand side does not exist", func() {
					data := dorun(func(f io.WriteCloser) {
						Run([]string{"testdata/input1.yaml"}, "", []string{}, []string{"newval = \"hello\""}, f)
					})

					docs := decodeDocs(data)

					Expect(docs[0]["newval"]).To(Equal("hello"))
				})

				It("should error if key on right hand side does not exist", func() {
					var e error
					dorun(func(f io.WriteCloser) {
						e = Run([]string{"testdata/input1.yaml"}, "", []string{}, []string{"maptype = noexist"}, f)
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("noexist: key does not exist"))
				})
			})

			Describe("unset", func() {
				PIt("should unset field if action is unset")
				PIt("should ignore if key does not exist")
				PIt("should error if argument count != 1")
			})

			Describe("merge", func() {
				PIt("should deep merge two documents")
				PIt("should error if types are not maps")
				PIt("should error if argument count != 2")
			})

			Describe("yaml", func() {
				PIt("should parse yaml string provided")
				PIt("should load yaml from filesystem if file exists")
				PIt("should error if parse error")
				PIt("should error if argument count != 1")
			})

			Describe("__root", func() {
				PIt("should return return root document")
				PIt("should set key at root if used on left hand side of expression")
				PIt("should allow merging with root of document")
			})

			Describe("b64encode", func() {
				PIt("should base64encode input")
				PIt("should error on problem with encode")
				PIt("should error if argument count != 1")
			})

			Describe("b64decode", func() {
				PIt("should bas64decode input")
				PIt("should error on problem with decode")
				PIt("should error if argument count != 1")
			})
		})
	})
})
