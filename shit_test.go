package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func TestAddAll(t *testing.T) {
	initt(t)

	// Add a file
	fileFixture("file1.txt", "A test")       // Hash = c4a5964fd224738514ccd7354a45d37a5ef1a8b3
	fileFixture("file2.txt", "Hello")        // Hash = be12174911e3aae8c2ed6ef5cb66b32893b3bd21
	fileFixture("a/b/c/file3.txt", "A test") // Hash = c4a5964fd224738514ccd7354a45d37a5ef1a8b3
	fileFixture("a/b/c/file4.txt", "Hello")  // Hash = be12174911e3aae8c2ed6ef5cb66b32893b3bd21

	run("add", "-A")

	expectedBowl := `c4a5964fd224738514ccd7354a45d37a5ef1a8b3 a/b/c/file3.txt
be12174911e3aae8c2ed6ef5cb66b32893b3bd21 a/b/c/file4.txt
c4a5964fd224738514ccd7354a45d37a5ef1a8b3 file1.txt
be12174911e3aae8c2ed6ef5cb66b32893b3bd21 file2.txt`
	bowl := getFile(".shit/bowl")
	assert(t, bowl, expectedBowl)

}

// Test removing a file from worktree, running add -A, expecting the removed file to be removed from the bowl, finally
func TestRemoveFromBowl(t *testing.T) {
	initt(t)

	// Add a file
	fileFixture("file1.txt", "A test") // Hash = c4a5964fd224738514ccd7354a45d37a5ef1a8b3
	fileFixture("file2.txt", "Hello")  // Hash = be12174911e3aae8c2ed6ef5cb66b32893b3bd21

	run("add", "-A")

	expectedBowl := `c4a5964fd224738514ccd7354a45d37a5ef1a8b3 file1.txt
be12174911e3aae8c2ed6ef5cb66b32893b3bd21 file2.txt`
	bowl := getFile(".shit/bowl")
	assert(t, bowl, expectedBowl)

	os.Remove("file2.txt")

	run("add", "-A")

	expectedBowl = `c4a5964fd224738514ccd7354a45d37a5ef1a8b3 file1.txt`
	bowl = getFile(".shit/bowl")
	assert(t, bowl, expectedBowl)
}

func TestGetBowl(t *testing.T) {
	initt(t)

	objectFixture("file\n\nA test") // Hash = c4a5964fd224738514ccd7354a45d37a5ef1a8b3
	fileFixture(".shit/bowl", `c4a5964fd224738514ccd7354a45d37a5ef1a8b3 file1.txt
`)

	bowl := getBowl()
	assert(t, bowl[0].Object.Hash, "c4a5964fd224738514ccd7354a45d37a5ef1a8b3")
}

func TestCreateObject(t *testing.T) {
	initt(t)
	createObject("file", "A test file\nWith two lines\n")
	assertObject(t, "197fa33f64bfce7ac12607ad567ea8573a38a823", "file\n\nA test file\nWith two lines\n")
}

func TestGetObject(t *testing.T) {
	initt(t)

	hash := objectFixture("file\n\nA test file\nWith two lines\n")
	output := run("get-object", hash)
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

	output := run("create-tree")
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

func TestFlush(t *testing.T) {
	initt(t)

	objectFixture("file\n\nA test") // Hash = c4a5964fd224738514ccd7354a45d37a5ef1a8b3
	objectFixture("file\n\nHello")  // Hash = be12174911e3aae8c2ed6ef5cb66b32893b3bd21

	bowl := `c4a5964fd224738514ccd7354a45d37a5ef1a8b3 file1.txt
be12174911e3aae8c2ed6ef5cb66b32893b3bd21 file2.txt
c4a5964fd224738514ccd7354a45d37a5ef1a8b3 dir1/file3.txt
be12174911e3aae8c2ed6ef5cb66b32893b3bd21 dir1/file4.txt
`
	fileFixture(".shit/bowl", bowl)

	output := run("flush", "-m", "A flush")
	cmtHash := strings.Split(strings.Split(output, "\n")[0], " ")[2]
	assert(t, getFile(".shit/bowl"), bowl)

	head := getFile(".shit/HEAD")
	assert(t, head, cmtHash)

	flush := getObject(head)
	content := string(flush.Bytes)
	assertLine(t, content, 0, "flush")
	assertLine(t, content, 1, "")
	assertLine(t, content, 2, "tree 842e8f2250e0bfd81fda08a61f2b874012ed5942")
	assertLine(t, content, 3, "parent ")
	assertLine(t, content, 6, "A flush")
}

func TestAddAndFlushMultipleTimes(t *testing.T) {
	initt(t)

	// Create commit with one file
	fileFixture("file1.txt", "File 1")
	run("add", "file1.txt")
	output := run("flush", "-m", "A flush")
	cmt1Hash := hashFromFlushOutput(output)

	// Add another file and commit
	fileFixture("file2.txt", "File 2")
	run("add", "file2.txt")
	output = run("flush", "-m", "Another flush")
	cmt2Hash := hashFromFlushOutput(output)

	// Change one file and commit
	fileFixture("file2.txt", "File 2 changed")
	run("add", "file2.txt")
	output = run("flush", "-m", "A third flush")
	cmt3Hash := hashFromFlushOutput(output)

	flush1 := getObject(cmt1Hash).ToFlush()
	flush2 := getObject(cmt2Hash).ToFlush()
	flush3 := getObject(cmt3Hash).ToFlush()

	assert(t, flush1.ParentHash, "")
	assert(t, flush2.ParentHash, cmt1Hash)
	assert(t, flush3.ParentHash, cmt2Hash)

	tree1 := getTree(flush1.TreeHash)
	tree2 := getTree(flush2.TreeHash)
	tree3 := getTree(flush3.TreeHash)

	assertInt(t, len(tree1.Nodes), 1)
	assert(t, tree1.Nodes[0].Name, "file1.txt")

	assertInt(t, len(tree2.Nodes), 2)
	assert(t, tree2.Nodes[0].Name, "file1.txt")
	assert(t, tree2.Nodes[1].Name, "file2.txt")

	assertInt(t, len(tree2.Nodes), 2)
	assert(t, tree3.Nodes[0].Name, "file1.txt")
	assert(t, tree3.Nodes[1].Name, "file2.txt")

	// File 2 should be different in commit 2 and 3
	if tree2.Nodes[1].Hash == tree3.Nodes[1].Hash {
		t.Error("fuck")
	}
}

func hashFromFlushOutput(output string) string {
	cmtHash := strings.Split(strings.Split(output, "\n")[0], " ")[2]
	return cmtHash
}

func run(command ...string) string {
	os.Args = []string{""}
	os.Args = append(os.Args, command...)
	w, r, o := startCaptureStdout()
	main()
	return endCaptureStdout(w, r, o)
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

func assertLine(t *testing.T, actual string, line_num int, expected string) {
	line := strings.Split(actual, "\n")[line_num]
	if line != expected {
		t.Errorf("Line %d did not match expected value. expected [%s], actual [%s]", line_num, expected, line)
	}
}

func assertInt(t *testing.T, actual int, expected int) {
	if actual != expected {
		t.Errorf("Expected %d but was %d", expected, actual)
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

func fileFixture(filePath string, content string) string {
	dir, _ := filepath.Split(filePath)
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}

	file, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}
	files = append(files, filePath)
	file.Write([]byte(content))
	file.Close()
	return hash([]byte(content))
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
func startCaptureStdout() (r *os.File, w *os.File, o *os.File) {
	o = os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}

	os.Stdout = w
	return r, w, o
}

func endCaptureStdout(r *os.File, w *os.File, o *os.File) string {
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()
	os.Stdout = o
	return buf.String()
}
