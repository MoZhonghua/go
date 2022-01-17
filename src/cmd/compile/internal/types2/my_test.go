package types2_test

import (
	"cmd/compile/internal/syntax"
	. "cmd/compile/internal/types2"
	"testing"
)

func TestInterface(t *testing.T) {
	mode := syntax.AllowGenerics

	var errlist []error
	errh := func(err error) { errlist = append(errlist, err) }
	file, err := syntax.ParseFile("/home/mozhonghua/go/src/playground/gotest/generic_interface/main.go", errh, nil, mode)
	if file == nil {
		t.Fatalf("%s: %s", filename, err)
	}

	if err != nil {
		t.Fatal(err)
	}

	var conf Config
	conf.GoVersion = "go1.18"
	// special case for importC.src
	conf.Trace = false
	conf.Importer = defaultImporter()
	conf.Error = func(err error) {
		errlist = append(errlist, err)
	}
	conf.Check("test", []*syntax.File{file}, nil)

	if len(errlist) != 0 {
		t.Fatalf("%v\n", errlist)
	}
}
