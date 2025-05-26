package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

var files = []string{}

func TestInit(t *testing.T) {
	initt(t)
	assertFile(t, ".shit/HEAD", "")
	assertFile(t, ".shit/bowl", "")
	assertDir(t, ".shit/objects", "")
}

func TestAdd(t *testing.T) {
	initt(t)

	// Add a file
	fileFixture("test.txt", "A test file\nWith two lines\n")
	run("add", "test.txt")

	assertFile(t, ".shit/bowl", "197fa33f64bfce7ac12607ad567ea8573a38a823 test.txt")
	assertDir(t, ".shit/objects", "197fa33f64bfce7ac12607ad567ea8573a38a823")
	assertObject(t, "197fa33f64bfce7ac12607ad567ea8573a38a823", "file\n\nA test file\nWith two lines\n")

	// Add another file
	fileFixture("other.txt", "Another file")
	run("add", "other.txt")

	assertFile(t, ".shit/bowl", "aff9a3a04647a47feed6d1c64e023397daff1191 other.txt\n197fa33f64bfce7ac12607ad567ea8573a38a823 test.txt")
	assertDir(t, ".shit/objects", "197fa33f64bfce7ac12607ad567ea8573a38a823\naff9a3a04647a47feed6d1c64e023397daff1191")

	// Update an existing file
	fileFixture("other.txt", "yet another file")
	run("add", "other.txt")

	assertFile(t, ".shit/bowl", "caa2b67db4872c7027aff70c5f7676ee3417ad50 other.txt\n197fa33f64bfce7ac12607ad567ea8573a38a823 test.txt")
	assertDir(t, ".shit/objects", "197fa33f64bfce7ac12607ad567ea8573a38a823\naff9a3a04647a47feed6d1c64e023397daff1191\ncaa2b67db4872c7027aff70c5f7676ee3417ad50")
}

func TestCreateObject(t *testing.T) {
	initt(t)
	objectFixture("file\n\nA test file\nWith two lines\n")
	assertObject(t, "197fa33f64bfce7ac12607ad567ea8573a38a823", "file\n\nA test file\nWith two lines\n")
}

func TestGetObject(t *testing.T) {
	initt(t)

	hash := objectFixture("file\n\nA test file\nWith two lines\n")
	r, w := recordStdout()

	run("get-object", hash)

	output := captureStdout(r, w)
	assert(t, output, `file

A test file
With two lines
`)
}

func TestCreateTree(t *testing.T) {
	initt(t)

	objectFixture("file\n\nA test") // Hash = c4a5964fd224738514ccd7354a45d37a5ef1a8b3

	fileFixture(".shit/bowl", `c4a5964fd224738514ccd7354a45d37a5ef1a8b3 file1.txt
c4a5964fd224738514ccd7354a45d37a5ef1a8b3 file2.txt
c4a5964fd224738514ccd7354a45d37a5ef1a8b3 dir1/file3.txt
c4a5964fd224738514ccd7354a45d37a5ef1a8b3 dir1/file4.txt
`)

	r, w := recordStdout()
	run("create-tree")
	output := captureStdout(r, w)
	parts := strings.Split(output, " ")

	assert(t, parts[0]+" "+parts[1], "Created tree")

	treeHash := strings.ReplaceAll(parts[2], "\n", "")
	object := getObject(treeHash)

	assert(t, object.Header.ObjectType, "tree")
	tree := object.ToTree()

	assert(t, tree.Nodes[0].NodeType, "tree")
	assert(t, tree.Nodes[0].Name, "dir1/")

	assert(t, tree.Nodes[1].NodeType, "file")
	assert(t, tree.Nodes[1].Name, "file1.txt")

	assert(t, tree.Nodes[2].NodeType, "file")
	assert(t, tree.Nodes[2].Name, "file2.txt")
}

//func TestFlush(t *testing.T) {
//	clean()
//	initt()
//
//	// Given
//	createFile("test.txt", "A test file\nWith two lines\n")
//	createObject("file", "A test file\nWith two lines\n")
//	createFile(".shit/bowl", "add 197fa33f64bfce7ac12607ad567ea8573a38a823 test.txt\n")
//
//	run("flush", "-m", "A flush")
//
//	head := getFile(".shit/HEAD")
//	flush := getObject(head)
//	content := string(flush.Bytes)
//	expectLine(t, content, 0, "flush")
//	expectLine(t, content, 1, "")
//	expectLine(t, content, 3, "parent ")
//	expectLine(t, content, 6, "A flush")
//
//	treeHash := strings.Split(strings.Split(content, "\n")[4], " ")[1]
//	assertObject(t, treeHash, "tree\n\n197fa33f64bfce7ac12607ad567ea8573a38a823 test.txt")
//}

func run(command ...string) {
	os.Args = []string{""}
	os.Args = append(os.Args, command...)
	main()
}

func assert(t *testing.T, actual string, expected string) {
	if actual != expected {
		t.Errorf("File did not match expectation.\n\nExpected:\n----------------------------------------\n%s\n----------------------------------------\n\nActual:\n----------------------------------------\n%s\n----------------------------------------\n\n", expected, actual)
	}
}

func assertFile(t *testing.T, filepath string, expected string) {
	actual := getFile(filepath)
	assert(t, actual, expected)
}

func assertDir(t *testing.T, path string, expected string) {
	actual := getDir(path)
	assert(t, actual, expected)
}

func assertObject(t *testing.T, hash string, expected string) {
	var reader, err = os.Open(".shit/objects/" + hash)
	if err != nil {
		panic(err)
	}
	var actual = string(decompress(reader))
	assert(t, actual, expected)
}

func expectLine(t *testing.T, actual string, line_num int, expected string) {
	line := strings.Split(actual, "\n")[line_num]
	if line != expected {
		t.Errorf("Line %d did not match expected value. expected %s, actual %s", line_num, expected, actual)
	}
}

func initt(t *testing.T) {
	dir := fmt.Sprintf("/tmp/%s_%s", hash([]byte(t.Name())), t.Name())
	os.RemoveAll(dir)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		panic(err)
	}
	err = os.Chdir(dir)
	if err != nil {
		panic(err)
	}
	fmt.Println("Running " + dir)
	os.Args = []string{"", "init"}
	main()
}

func fileFixture(name string, content string) {
	file, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	files = append(files, name)
	file.Write([]byte(content))

}

func objectFixture(content string) string {
	compressed := compress([]byte(content))
	hash := hash([]byte(content))
	fileFixture(".shit/objects/"+hash, compressed.String())
	return hash
}

func getFile(path string) string {
	bytes, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func getDir(path string) string {
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		panic(err)
	}
	dir := []string{}

	for _, dirEntry := range dirEntries {
		dir = append(dir, dirEntry.Name())
	}
	return strings.Join(dir, "\n")
}

// r and w must be closed
func recordStdout() (r *os.File, w *os.File) {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}

	os.Stdout = w
	return r, w
}

func captureStdout(r *os.File, w *os.File) string {
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()
	return buf.String()
}
