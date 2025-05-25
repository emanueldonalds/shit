package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
)

const SHIT_PATH = ".shit"
const BOWL_PATH = SHIT_PATH + "/bowl"
const OBJECTS_PATH = SHIT_PATH + "/objects"

type Command struct {
	Action string
	Args   []string
}

type Header struct {
	ObjectType string
	Len        int
}

type Object struct {
	ObjectType string
	Hash       string
	Bytes      []byte
}

type BowlEntry struct {
	Path   string
	Object *Object
}

func main() {
	var command *Command = parseArgs()

	if command.Action == "--help" {
		exitUsage()
	}

	checkInit(command.Action)

	switch command.Action {
	case "init":
		cmdInitShit()
	case "add":
		cmdAdd(command.Args)
	case "get-object":
		cmdGetObject(command.Args)
	case "flush":
		cmdFlush(command.Args)
	case "create-tree":
		cmdCreateTree()
	default:
		exitUsage()
	}
}

func parseArgs() *Command {
	if len(os.Args) < 2 {
		exitUsage()
	}

	return &Command{Action: os.Args[1], Args: os.Args[2:]}
}

func checkInit(action string) {
	_, err := os.Stat(SHIT_PATH)
	dirIsTracked := err == nil

	if !dirIsTracked && action == "init" {
		return
	} else if dirIsTracked && action == "init" {
		fmt.Println("Directory is already tracked by Shit, aborting init.")
		exitUsage()
	} else if !dirIsTracked {
		fmt.Println("Directory is not tracked by Shit, initialize dir with \"shit init\" first.")
		exitUsage()
	}

}

func cmdInitShit() {
	if err := os.Mkdir(SHIT_PATH, 0775); err != nil {
		panic(err)
	}

	if err := os.Mkdir(OBJECTS_PATH, 0775); err != nil {
		panic(err)
	}

	if _, err := os.Create(BOWL_PATH); err != nil {
		panic(err)
	}
}

func cmdAdd(args []string) {
	if len(args) < 1 {
		exitUsage()
	}

	bowl := getBowl()
	var paths []string

	if args[0] == "-A" {
		paths = getWorkdir()
	} else {
		paths = append(paths, args[0])
	}

	for _, path := range paths {
		object := createObject("file", readFile(path))
		bowlEntry := &BowlEntry{Object: object, Path: path}
		bowl = appendBowl(bowl, bowlEntry)
	}

	writeBowl(bowl)
}

func cmdGetObject(args []string) {
	if len(args) < 1 {
		exitUsage()
	}

	hash := args[0]
	object := getObject(hash)
	var headerLen int

	headerLen = getHeader(object).Len

	content := object[headerLen:]
	fmt.Print(content)
}

func cmdFlush(args []string) {
	if len(args) < 1 {
		fmt.Println("A message is required when flusing (-m <message>).")
		exitUsage()
	}

	fmt.Println("TBD")
}

func cmdCreateTree() {
	bowl := getBowl()
	tree := createTree(bowl)
	fmt.Println(string(tree.Bytes))
}

func getWorkdir() []string {
	var dir []string

	var walkDirFunc fs.WalkDirFunc = func(path string, d fs.DirEntry, err error) error {
		if strings.Contains(path, ".git") || strings.Contains(path, ".shit") {
			return nil
		}
		if d.Type().IsRegular() {
			dir = append(dir, path)
		}
		return nil
	}

	filepath.WalkDir(".", walkDirFunc)
	return dir
}

func getBowl() []*BowlEntry {
	bowlFile := readFile(BOWL_PATH)
	bowlLines := strings.Split(bowlFile, "\n")
	var bowl []*BowlEntry

	if bowlLines[0] == "" {
		return bowl
	}

	for _, line := range bowlLines {
		if line == "" {
			continue
		}

		lineParts := strings.Split(line, " ")
		object := &Object{ObjectType: lineParts[0], Hash: lineParts[1]}
		bowl = append(bowl, &BowlEntry{Object: object, Path: lineParts[2]})
	}

	return bowl
}

func writeBowl(bowl []*BowlEntry) {
	slices.SortFunc(bowl, func(a, b *BowlEntry) int {
		return strings.Compare(a.Path, b.Path)
	})
	var buf bytes.Buffer
	for _, bowlEntry := range bowl {
		buf.WriteString(fmt.Sprintf("%s %s %s\n", bowlEntry.Object.ObjectType, bowlEntry.Object.Hash, bowlEntry.Path))
	}
	writeFile(BOWL_PATH, buf)
}

func appendBowl(bowl []*BowlEntry, newEntry *BowlEntry) []*BowlEntry {
	var newBowl []*BowlEntry

	for _, oldEntry := range bowl {
		if oldEntry.Path != newEntry.Path {
			newBowl = append(newBowl, oldEntry)
		}
	}

	newBowl = append(newBowl, newEntry)
	return newBowl
}

func getObject(hash string) string {
	var objectPath = fmt.Sprintf(OBJECTS_PATH+"/%s", hash)
	var reader, err = os.Open(objectPath)
	if err != nil {
		panic(err)
	}

	return decompress(reader)
}

func createObject(objectType string, content string) *Object {
	var bytes []byte = addHeader(objectType, content)
	var hash string = hash(bytes)
	var objectPath = fmt.Sprintf(OBJECTS_PATH+"/%s", hash)

	writeFile(objectPath, compress(bytes))
	return &Object{ObjectType: objectType, Hash: hash, Bytes: bytes}
}

func getHeader(object string) *Header {
	var headerLen int
	line := 0
	for i, c := range object {
		if line == 2 {
			headerLen = i
			break
		}
		if c == '\n' {
			line = line + 1
		}
	}

	headerLines := strings.Split(object[:headerLen], "\n")
	return &Header{ObjectType: headerLines[0], Len: headerLen}
}

func addHeader(objectType string, content string) []byte {
	// Object format:

	// type
	// <empty-line>
	// content

	var parts = strings.Join([]string{objectType, "", content}, "\n")

	return []byte(parts)
}

func createTree(bowlEntries []*BowlEntry) *Object {
	tree := make(map[string]*Object)
	subtrees := make(map[string][]*BowlEntry)

	for _, bowlEntry := range bowlEntries {
		dir, file := filepath.Split(bowlEntry.Path)
		if dir == "" {
			tree[file] = bowlEntry.Object
		} else {
			bowlEntry.Path = file
			subtrees[dir] = append(subtrees[dir], bowlEntry)
		}
	}

	for dir, subtree := range subtrees {
		tree[dir] = createTree(subtree)
	}

	var sortedKeys []string
	for key, _ := range tree {
		sortedKeys = append(sortedKeys, key)
	}
	slices.Sort(sortedKeys)

	treeObjectBuf := new(strings.Builder)
	for _, key := range sortedKeys {
		treeObject := tree[key]
		treeObjectBuf.WriteString(fmt.Sprintf("%s %s %s\n", treeObject.ObjectType, treeObject.Hash, key))
	}

	return createObject("tree", treeObjectBuf.String())
}

func readFile(path string) string {
	var bytes, err = os.ReadFile(path)

	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func writeFile(path string, buf bytes.Buffer) {
	file, err := os.Create(path)

	if err != nil {
		panic(err)
	}

	defer file.Close()

	_, err = io.Copy(file, &buf)

	if err != nil {
		panic(err)
	}
}

func hash(bytes []byte) string {
	hasher := sha1.New()
	hasher.Write(bytes)
	return hex.EncodeToString(hasher.Sum(nil))
}

func compress(b []byte) bytes.Buffer {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf
}

func decompress(r io.Reader) string {
	buf := new(strings.Builder)

	decompressed, err := zlib.NewReader(r)
	if err != nil {
		panic(err)
	}

	io.Copy(buf, decompressed)
	decompressed.Close()
	return buf.String()
}

func exitUsage() {
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "Usage:\n\nshit init\tInitialize Shit repository\nshit add <filename>\tAdd a file to the the index\nshit flush <filename>\tWrite the current index to a commit\nshit sniff\tShow the current status of the index\tshit get-object\tGet an object from the object store")
	w.Flush()
	os.Exit(0)
}
