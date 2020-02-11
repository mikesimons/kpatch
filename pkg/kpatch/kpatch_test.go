package kpatch

import (
	"bytes"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/ansel1/merry"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
	yaml "gopkg.in/yaml.v2"
)

type RunParams struct {
	Files    []string
	Selector string
	Merges   []string
	Actions  []string
	Params   []string
}

func DefaultRunParams() RunParams {
	return RunParams{
		Files: []string{"testdata/input1.yaml", "testdata/input2.yaml"},
	}
}

func TestKpatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kpatch Suite")
}

func dorun(fn func(rp *RunParams)) ([]byte, error) {
	fs := afero.NewMemMapFs()
	f, _ := fs.OpenFile("output.yaml", os.O_CREATE|os.O_WRONLY, os.ModeAppend)

	rp := DefaultRunParams()
	fn(&rp)

	e := Run(rp.Files, rp.Selector, rp.Merges, rp.Actions, rp.Params, f)
	f.Close()

	f, _ = fs.Open("output.yaml")
	defer f.Close()

	data, _ := ioutil.ReadAll(f)

	return data, e
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

	Describe("Reset", func() {
		It("should reset all internal state", func() {
			k := &kpatch{
				targets:        make([]tTarget, 2),
				drop:           true,
				missingKeyMode: "xxx",
				doc:            map[interface{}]interface{}{"XXX": "XXX"},
				currentItem:    "one",
			}

			k.Reset()

			Expect(k.targets).To(HaveLen(0))
			Expect(k.drop).To(BeFalse())
			Expect(k.missingKeyMode).To(Equal("get"))
			Expect(k.doc).To(HaveLen(0))
			Expect(k.currentItem).To(BeNil())
		})
	})

	Describe("Run", func() {
		It("should process multiple inputs with multiple documents in each", func() {
			data, e := dorun(func(rp *RunParams) {})

			Expect(e).To(BeNil())
			docs := decodeDocs(data)
			Expect(docs).To(HaveLen(4))
		})

		It("should apply all actions to documents that match the selector", func() {
			data, e := dorun(func(rp *RunParams) {
				rp.Selector = `name =~ ".*document2"`
				rp.Actions = []string{`test1 = 1`, `test2 = 2`}
			})

			Expect(e).To(BeNil())
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
			data, e := dorun(func(rp *RunParams) {
				rp.Selector = `false`
			})

			Expect(e).To(BeNil())
			docs := decodeDocs(data)
			Expect(docs).To(HaveLen(4))
		})

		Describe("selector", func() {
			It("should only match documents that match expression", func() {
				data, e := dorun(func(rp *RunParams) {
					rp.Selector = `name =~ ".*document1"`
					rp.Actions = []string{`drop`}
				})

				Expect(e).To(BeNil())
				docs := decodeDocs(data)

				Expect(docs).To(HaveLen(2))
				Expect(docs[0]["name"]).To(Equal("input1document2"))
				Expect(docs[1]["name"]).To(Equal("input2document2"))
			})

			It("should not allow assignments in selector", func() {
				_, e := dorun(func(rp *RunParams) {
					rp.Selector = `test = 123`
				})

				Expect(e).NotTo(BeNil())
				Expect(merry.UserMessage(e)).To(ContainSubstring("unexpected \"=\""))
			})

			// TODO: Expand to other mutating functions (e.g. merge)
			It("should not allow mutating functions in selector", func() {
				_, e := dorun(func(rp *RunParams) {
					rp.Selector = `unset(name)`
				})

				Expect(e).NotTo(BeNil())
				Expect(merry.UserMessage(e)).To(ContainSubstring("could not call 'unset'"))
			})

			It("should match all documents if not specified", func() {
				data, e := dorun(func(rp *RunParams) {
					rp.Actions = []string{`drop`}
				})

				Expect(e).To(BeNil())
				docs := decodeDocs(data)
				Expect(docs).To(HaveLen(0))
			})
		})

		Describe("Action language", func() {
			Describe("drop", func() {
				It("should drop matching documents", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Selector = `name == "input1document1"`
						rp.Actions = []string{`drop`}
					})

					Expect(e).To(BeNil())
					docs := decodeDocs(data)
					names := make([]string, 0)
					for _, doc := range docs {
						names = append(names, doc["name"].(string))
					}

					Expect(names).To(HaveLen(3))
					Expect(names).NotTo(ContainElement("input1document1"))
				})
			})

			Describe("assign", func() {
				It("should set field if action is assignment", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`maptype = "hello"`}
					})

					Expect(e).To(BeNil())
					docs := decodeDocs(data)

					Expect(docs[0]["maptype"]).To(Equal("hello"))
				})

				It("should create key if key on left hand side does not exist", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`newval = "hello"`}
					})

					Expect(e).To(BeNil())
					docs := decodeDocs(data)

					Expect(docs[0]["newval"]).To(Equal("hello"))
				})

				It("should assign nil if key on right hand side does not exist", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`maptype = noexist`}
					})

					Expect(e).To(BeNil())
					docs := decodeDocs(data)

					Expect(docs[0]["maptype"]).To(BeNil())
				})
			})

			Describe("unset", func() {
				It("should unset field", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`unset(name)`}
					})

					docs := decodeDocs(data)

					Expect(e).To(BeNil())
					Expect(docs[0]["name"]).To(BeNil())
				})

				It("should unset multiple fields", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`unset(name, maptype)`}
					})

					docs := decodeDocs(data)

					Expect(e).To(BeNil())
					Expect(docs[0]["name"]).To(BeNil())
					Expect(docs[0]["maptype"]).To(BeNil())
				})

				It("should unset array elements", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`list | if(@ > 1, @) | unset(@)`}
					})

					docs := decodeDocs(data)

					Expect(e).To(BeNil())
					Expect(docs[1]["list"]).To(HaveLen(1))
				})

				It("should not do anything if key does not exist", func() {
					before, e := dorun(func(rp *RunParams) {})
					Expect(e).To(BeNil())

					after, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`unset(n)`}
					})

					Expect(e).To(BeNil())
					Expect(before).To(Equal(after))
				})

				It("should error if argument count < 1", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`unset()`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("unset(var, ...) requires one or more argument to unset"))
				})
			})

			Describe("@ variable", func() {
				It("should return root outside of pipeline", func() {
					before, e := dorun(func(rp *RunParams) {})
					Expect(e).To(BeNil())

					after, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`tmp = @`}
					})

					docsBefore := decodeDocs(before)
					docs := decodeDocs(after)

					Expect(e).To(BeNil())
					Expect(docs[0]["tmp"]).NotTo(BeNil())
					Expect(docs[0]["tmp"]).To(Equal(docsBefore[0]))
				})

				It("should return current item inside pipeline", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Selector = `name == "input1document2"`
						rp.Actions = []string{`list | @ = "X"`}
					})

					docs := decodeDocs(data)

					Expect(e).To(BeNil())
					Expect(docs[1]["list"]).To(HaveLen(3))

					var items []string
					for _, i := range docs[1]["list"].([]interface{}) {
						items = append(items, i.(string))
					}
					Expect(strings.Join(items, ".")).To(Equal("X.X.X"))
				})
			})

			Describe("v", func() {
				It("should return var at path by args", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Selector = `name == "input1document1"`
						rp.Actions = []string{`test = v(["maptype", "k1", "k1"])`}
					})

					docs := decodeDocs(data)

					Expect(e).To(BeNil())
					Expect(docs[0]["test"]).To(Equal("l2value"))
				})

				It("should return error if arg is not a slice", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Selector = `name == "input1document1"`
						rp.Actions = []string{`test = v("test")`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("v(path) expects path to be a slice"))
				})

				It("should return error if path invalid", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Selector = `name == "input1document1"`
						rp.Actions = []string{`v(["test"])`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("key does not exist"))
				})

				It("should return error if argument count != 1", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Selector = `name == "input1document1"`
						rp.Actions = []string{`v(["test"], "other")`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("requires exactly one argument"))
				})
			})

			Describe("merge", func() {
				PIt("should deep merge two documents")
				PIt("should error if types are not maps")
				PIt("should error if argument count < 2")
			})

			Describe("yaml_parse", func() {
				It("should parse yaml string provided", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = yaml_parse("{ key: val }")`}
					})

					Expect(e).To(BeNil())

					docs := decodeDocs(data)
					Expect(docs[0]["test"]).NotTo(BeNil())
					test := docs[0]["test"].(map[interface{}]interface{})
					Expect(test["key"]).To(Equal("val"))
				})

				It("should load yaml from filesystem if file exists", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = yaml_parse("testdata/yaml.yaml")`}
					})

					Expect(e).To(BeNil())

					docs := decodeDocs(data)
					Expect(docs[0]["test"]).NotTo(BeNil())
					test := docs[0]["test"].(map[interface{}]interface{})
					Expect(test["test"]).To(Equal(1234))
				})

				It("should error on a parse error", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = yaml_parse("{{{ x")`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("yaml: line 1: did not find expected"))
				})

				It("should error if argument is not a string", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`yaml_parse([])`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("expects input to be a string"))
				})

				It("should error if argument count != 1", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`yaml_parse()`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("requires exactly one argument"))
				})
			})

			Describe("yaml_dump", func() {
				PIt("should return variable as yaml string provided")
				PIt("should error if input can't be marshalled")
				PIt("should error if argument count != 1")
			})

			Describe("b64encode", func() {
				It("should base64 encode input", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = b64encode("test")`}
					})

					Expect(e).To(BeNil())

					docs := decodeDocs(data)
					Expect(docs[0]["test"]).To(Equal("dGVzdA=="))
				})

				It("should error if argument count != 1", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = b64encode()`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("requires exactly one argument"))
				})
			})

			Describe("b64decode", func() {
				It("should base64 decode input", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = b64decode("dGVzdA==")`}
					})

					Expect(e).To(BeNil())

					docs := decodeDocs(data)
					Expect(docs[0]["test"]).To(Equal("test"))
				})

				It("should error on problem with decode", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = b64decode("XXXXXXX")`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("illegal base64 data"))
				})

				It("should error if argument count != 1", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = b64decode()`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("requires exactly one argument"))
				})

				It("should error if first argument is not a string", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = b64decode([])`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("expects input to be a string"))
				})
			})

			Describe("pipe operator", func() {
				PIt("should invoke RHS for each element of LHS")
				PIt("should make LHS a slice if it is not already")
				PIt("should not onvoke RHS for nil elements")
			})

			Describe("if", func() {
				It("should return the second argument if the first is true", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = if(true, "first", "second")`}
					})

					Expect(e).To(BeNil())

					docs := decodeDocs(data)
					Expect(docs[0]["test"]).To(Equal("first"))
				})

				It("should return the third argument if the first is not true", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = if(false, "first", "second")`}
					})

					Expect(e).To(BeNil())

					docs := decodeDocs(data)
					Expect(docs[0]["test"]).To(Equal("second"))
				})

				It("should accept two arguments and return nil if the first is not true", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = if(false, "first")`}
					})

					Expect(e).To(BeNil())

					docs := decodeDocs(data)
					Expect(docs[0]["test"]).To(BeNil())
				})

				It("should error if argument count > 3", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = if(false, "first", "second", "third")`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("if(cond, istrue, [isfalse]) takes 2 or 3 arguments"))
				})

				It("should error if argument count < 2", func() {
					_, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = if(false)`}
					})

					Expect(e).NotTo(BeNil())
					Expect(merry.UserMessage(e)).To(ContainSubstring("if(cond, istrue, [isfalse]) takes 2 or 3 arguments"))
				})
			})

			Describe("nil", func() {
				It("should return nil", func() {
					data, e := dorun(func(rp *RunParams) {
						rp.Actions = []string{`test = nil()`}
					})

					Expect(e).To(BeNil())

					docs := decodeDocs(data)
					Expect(docs[0]["test"]).To(BeNil())
				})
			})

			Describe("prefix", func() {
				PIt("should add prefix to input if not present")
				PIt("should not prefix input if prefix already present")
				PIt("should error if args are not strings")
				PIt("should error if argument count < 2")
				PIt("should error if argument count > 2")
			})

			Describe("suffix", func() {
				PIt("should add suffix to input if not present")
				PIt("should not suffix input if suffix already present")
				PIt("should error if args are not strings")
				PIt("should error if argument count < 2")
				PIt("should error if argument count > 2")
			})

			Describe("split", func() {
				PIt("should split input on separator")
				PIt("should return input in slice if separator not present")
				PIt("should error if argument count < 2")
				PIt("should error if argument count > 2")
			})

			Describe("join", func() {
				PIt("should join input with separator")
				PIt("should return input as string if input is slice with single item")
				PIt("should error if argument count < 2")
				PIt("should error if argument count > 2")
			})

			Describe("upper", func() {
				PIt("should uppercase input")
			})

			Describe("lower", func() {
				PIt("should lowercase input")
			})

			Describe("concat", func() {
				PIt("should concatenate strings")
				PIt("should append slices")
				PIt("should error if slices are incompatible types")
			})

			Describe("splice_replace", func() {
				PIt("should inject multiple slice elements in place of the target item")
			})
		})
	})
})
