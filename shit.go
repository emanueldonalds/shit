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
	"time"
)

const SHIT_PATH = ".shit"
const HEAD_PATH = SHIT_PATH + "/HEAD"
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
	Header     *Header //TODO pointers in structs?
	Content    string
	Bytes      []byte
}

type BowlEntry struct {
	Change string
	Object *Object
	Path   string
}

type Flush struct {
	Object     *Object //TODO polymorphism?
	Date       string
	ParentHash string
	TreeHash   string
	Message    string
}

type Tree struct {
	Object *Object
	Nodes  []TreeNode
}

type TreeNode struct {
	Name       string
	ObjectType string // file or dir
	Hash       string
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
	create := func(t string, path string) {
		var err error
		if t == "dir" {
			err = os.Mkdir(path, 0775)
		} else {
			_, err = os.Create(path)
		}
		if err != nil {
			panic(err)
		}
	}
	create("dir", SHIT_PATH)
	create("dir", OBJECTS_PATH)
	create("file", BOWL_PATH)
	create("file", HEAD_PATH)
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

	head := getHead()

	for _, path := range paths {
		treeNode := findNodeInFlush(head, path)
		wdFile := findFileInWorkdir(path)
		treeNodeExists := treeNode != nil
		wdFileExists := wdFile != nil

		change := "add"
		if treeNodeExists && wdFileExists {
			change = "edit"
		} else if treeNodeExists && !wdFileExists {
			change = "delete"
		}

		var object *Object

		if change == "delete" {
			object = treeNode
		} else {
			object = createObject("file", readFile(path))
		}

		bowlEntry := &BowlEntry{Object: object, Path: path, Change: change}
		bowl = appendBowl(bowl, bowlEntry)
	}

	writeBowl(bowl)
}

func findFileInWorkdir(path string) os.FileInfo {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return info
}

func cmdGetObject(args []string) {
	if len(args) < 1 {
		exitUsage()
	}

	hash := args[0]
	object := getObject(hash)
	fmt.Print(string(object.Bytes))
}

func cmdFlush(args []string) {
	if len(args) < 2 || args[0] != "-m" {
		fmt.Println("A message is required when flusing (-m <message>).")
		exitUsage()
	}

	message := args[1]

	bowl := getBowl()
	if len(bowl) == 0 {
		fmt.Println("Your bowl is empty, add files to bowl with \"flush add <filename>\" first.")
		exitUsage()
	}

	tree := createTree(bowl)
	parent := getHead()
	createFlush(tree, parent, message)
}

func createFlush(tree *Object, parent *Flush, message string) {
	flushContent := new(strings.Builder)

	flushContent.WriteString("time " + time.Now().UTC().String() + "\n")
	flushContent.WriteString("parent ")
	if parent != nil {
		flushContent.WriteString(parent.Object.Hash)
	}
	flushContent.WriteString("\n")

	flushContent.WriteString("tree " + tree.Hash + "\n")
	flushContent.WriteString("\n")
	flushContent.WriteString(message)

	flush := createObject("flush", flushContent.String())

	fmt.Println("Created flush " + flush.Hash + "\n============================")
	fmt.Println(string(flush.Bytes))

	// Update head
	err := os.WriteFile(SHIT_PATH+"/HEAD", []byte(flush.Hash), 0644)
	if err != nil {
		panic(err)
	}

	// Clear bowl
	f := new(bytes.Buffer)
	writeFile(BOWL_PATH, *f)
}

func getHead() *Flush {
	headFile, err := os.ReadFile(HEAD_PATH)
	if err != nil {
		panic(err)
	}

	hash := string(headFile)
	if len(hash) < 40 { //TODO what is sha1 min length?
		return nil
	}

	return getFlush(hash)
}

func getFlush(hash string) *Flush {
	object := getObject(hash)

	lines := strings.Split(object.Content, "\n")
	date := strings.Split(lines[0], " ")[1]
	parent := strings.Split(lines[1], " ")[1]
	tree := strings.Split(lines[2], " ")[1]
	message := strings.Join(lines[4:], "\n")

	return &Flush{Object: object, Date: date, ParentHash: parent, TreeHash: tree, Message: message}
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
		change := lineParts[0]
		hash := lineParts[1]
		path := lineParts[2]
		object := getObject(hash)
		bowl = append(bowl, &BowlEntry{Change: change, Object: object, Path: path})
	}

	return bowl
}
func findNodeInFlush(flush *Flush, path string) *Object {
	if flush == nil {
		return nil
	}
	tree := getTree(flush.TreeHash)
	return findNode(tree, path)
}

func findNode(tree *Tree, path string) *Object {
	dir, filepath := filepath.Split(path)

	if dir == "" {
		// Look for a file
		for _, node := range tree.Nodes {
			if node.Name == filepath {
				return getObject(node.Hash)
			}
		}
		return nil
	}

	// Look for a tree
	for _, node := range tree.Nodes {
		if node.Name == dir {
			nodeObject := getObject(node.Hash)
			if nodeObject.Header.ObjectType != "tree" {
				return nil
			}
			return findNode(toTree(nodeObject), filepath)
		}
	}

	return nil
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

func writeBowl(bowl []*BowlEntry) {
	slices.SortFunc(bowl, func(a, b *BowlEntry) int {
		return strings.Compare(a.Path, b.Path)
	})
	var buf bytes.Buffer
	for _, bowlEntry := range bowl {
		buf.WriteString(fmt.Sprintf("%s %s %s\n", bowlEntry.Change, bowlEntry.Object.Hash, bowlEntry.Path))
	}
	writeFile(BOWL_PATH, buf)
}

func getObject(hash string) *Object {
	var objectPath = fmt.Sprintf(OBJECTS_PATH+"/%s", hash)
	var reader, err = os.Open(objectPath)
	if err != nil {
		panic(err)
	}

	objectBytes := decompress(reader)
	header := getHeader(objectBytes)
	contentBytes := objectBytes[header.Len:]
	content := string(contentBytes)

	return &Object{Hash: hash, Header: header, Content: content, Bytes: objectBytes}

}

func createObject(objectType string, content string) *Object {
	var bytes []byte = addHeader(objectType, content)
	var hash string = hash(bytes)
	var objectPath = fmt.Sprintf(OBJECTS_PATH+"/%s", hash)

	writeFile(objectPath, compress(bytes))
	return &Object{ObjectType: objectType, Hash: hash, Bytes: bytes}
}

func getHeader(object []byte) *Header {
	objectStr := string(object)

	var headerLen int
	line := 0
	for i, c := range objectStr {
		if line == 2 {
			headerLen = i
			break
		}
		if c == '\n' {
			line = line + 1
		}
	}

	headerLines := strings.Split(objectStr[:headerLen], "\n")
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

func toTree(object *Object) *Tree {
	lines := strings.Split(object.Content, "\n")
	nodes := []TreeNode{}
	for _, line := range lines {
		if len(line) > 0 {
			parts := strings.Split(line, " ")
			nodes = append(nodes, TreeNode{Name: parts[2], ObjectType: parts[0], Hash: parts[1]})
		}
	}
	return &Tree{Object: object, Nodes: nodes}
}

func getTree(hash string) *Tree {
	object := getObject(hash)
	return toTree(object)
}

// This has to merge with existing HEAD tree somehow
func createTree(bowlEntries []*BowlEntry) *Object {
	treeNodes := []TreeNode{}
	subtrees := make(map[string][]*BowlEntry)

	for _, bowlEntry := range bowlEntries {
		dir, file := filepath.Split(bowlEntry.Path)
		if dir == "" {
			treeNodes = append(treeNodes, TreeNode{Name: file, ObjectType: "file", Hash: bowlEntry.Object.Hash})
		} else {
			bowlEntry.Path = file
			subtrees[dir] = append(subtrees[dir], bowlEntry)
		}
	}

	for dir, subtree := range subtrees {
		subtreeObject := createTree(subtree)
		treeNodes = append(treeNodes, TreeNode{Name: dir, ObjectType: "file", Hash: subtreeObject.Hash})
	}

	slices.SortFunc(treeNodes, func(a TreeNode, b TreeNode) int {
		return strings.Compare(a.Name, b.Name)
	})

	treeObjectBuf := new(strings.Builder)
	for _, treeNode := range treeNodes {
		treeObjectBuf.WriteString(fmt.Sprintf("%s %s %s\n", treeNode.ObjectType, treeNode.Hash, treeNode.Name))
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

func decompress(r io.Reader) []byte {
	decompressed, err := zlib.NewReader(r)
	if err != nil {
		panic(err)
	}

	var buf = new(bytes.Buffer)
	buf.ReadFrom(decompressed)
	decompressed.Close()
	return buf.Bytes()
}

func exitUsage() {
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "Usage:\n\nshit init\tInitialize Shit repository\nshit add <filename>\tAdd a file to the the bowl\nshit flush -m <message>\tWrite the current bowl to a flush\nshit sniff\tShow the current status of the bowl")
	w.Flush()
	os.Exit(0)
}
