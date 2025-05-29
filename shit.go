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
	Content    string
}

type Object struct {
	Hash    string
	Header  Header
	Content string
	Bytes   []byte
}

type BowlEntry struct {
	Change string
	Object Object
	Path   string
}

type Flush struct {
	Object     Object
	Date       string
	ParentHash string
	TreeHash   string
	Message    string
}

type Tree struct {
	Object Object
	Nodes  []TreeNode
}

func (object Object) ToTree() Tree {
	lines := strings.Split(object.Content, "\n")
	nodes := []TreeNode{}
	for _, line := range lines {
		if len(line) > 0 {
			parts := strings.Split(line, " ")
			nodes = append(nodes, TreeNode{Name: parts[2], NodeType: parts[0], Hash: parts[1]})
		}
	}
	return Tree{Object: object, Nodes: nodes}
}

func (object Object) ToFlush() Flush {
	lines := strings.Split(object.Content, "\n")
	tree := strings.Split(lines[0], " ")[1]
	parent := strings.Split(lines[1], " ")[1]
	date := strings.Split(lines[2], " ")[1]
	message := strings.Join(lines[4:], "\n")

	return Flush{Object: object, Date: date, ParentHash: parent, TreeHash: tree, Message: message}
}

type TreeNode struct {
	Name     string
	NodeType string // file or tree
	Hash     string
}

func main() {
	command := parseArgs()

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

func parseArgs() Command {
	if len(os.Args) < 2 {
		exitUsage()
	}

	return Command{Action: os.Args[1], Args: os.Args[2:]}
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
	createFs := func(t string, path string) {
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
	createFs("dir", SHIT_PATH)
	createFs("dir", OBJECTS_PATH)
	createFs("file", BOWL_PATH)
	createFs("file", HEAD_PATH)
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

		change := "add" // TODO Instead of storing this in the struct, just apply the change to the bowl...
		if treeNodeExists && wdFileExists {
			change = "edit"
		} else if treeNodeExists && !wdFileExists {
			change = "delete"
		}

		var object Object

		if change == "delete" {
			continue
		} else {
			object = createObject("file", readFile(path))
		}

		bowlEntry := BowlEntry{Object: object, Path: path, Change: change}
		bowl = mergeBowl(bowl, bowlEntry)
	}

	writeBowl(bowl)
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

func cmdCreateTree() {
	bowl := getBowl()
	tree := createTree(bowl)
	fmt.Println("Created tree " + tree.Object.Hash)
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

func getHead() *Flush {
	headFile, err := os.ReadFile(HEAD_PATH)
	if err != nil {
		panic(err)
	}

	hash := string(headFile)
	if len(hash) < 40 { // sha1 is 40 chars
		return nil
	}

	head := getFlush(hash)
	return &head
}

func getFlush(hash string) Flush {
	object := getObject(hash)
	return object.ToFlush()
}

func createFlush(tree Tree, parent *Flush, message string) {
	var parentHash string
	if parent != nil {
		parentHash = parent.Object.Hash
	}

	content := fmt.Sprintf(`tree %s
parent %s
time %s

%s
`, tree.Object.Hash, parentHash, time.Now().UTC().String(), message)

	flush := createObject("flush", content)

	// Update head
	err := os.WriteFile(SHIT_PATH+"/HEAD", []byte(flush.Hash), 0644)
	if err != nil {
		panic(err)
	}

	fmt.Println("Created flush " + flush.Hash + "\n======================================================")
}

func findNodeInFlush(flush *Flush, path string) *Object {
	if flush == nil {
		return nil
	}
	fmt.Println("Hash IS " + flush.TreeHash)
	tree := getTree(flush.TreeHash)
	return findNode(tree, path)
}

func findNode(tree Tree, path string) *Object {
	parts := strings.Split(path, string(filepath.Separator))

	if len(parts) > 1 {
		// Look for a tree
		dir := parts[0]
		for _, node := range tree.Nodes {
			if node.Name == dir {
				nodeObject := getObject(node.Hash)
				if nodeObject.Header.ObjectType != "tree" {
					return nil
				}
				childPath := strings.Join(parts[1:], string(filepath.Separator))
				return findNode(nodeObject.ToTree(), childPath)
			}
		}
	} else {
		// Look for a file
		fileName := parts[0]
		for _, node := range tree.Nodes {
			if node.Name == fileName {
				nodeObject := getObject(node.Hash)
				return &nodeObject
			}
		}
	}

	return nil
}

func getBowl() []BowlEntry {
	bowlFile := readFile(BOWL_PATH)
	bowlLines := strings.Split(bowlFile, "\n")

	var bowl []BowlEntry

	if bowlLines[0] == "" {
		return bowl
	}

	for _, line := range bowlLines {
		if line == "" {
			continue
		}

		cleaned := strings.TrimSpace(line)
		cleaned = strings.Trim(line, "\"\r\n")

		lineParts := strings.Split(cleaned, " ")
		hash := lineParts[0]
		path := lineParts[1]
		object := getObject(hash)
		bowl = append(bowl, BowlEntry{Object: object, Path: path})
	}

	return bowl
}

func mergeBowl(bowl []BowlEntry, newEntry BowlEntry) []BowlEntry {

	var newBowl []BowlEntry

	for _, oldEntry := range bowl {
		if oldEntry.Path != newEntry.Path {
			newBowl = append(newBowl, oldEntry)
		}
	}

	newBowl = append(newBowl, newEntry)
	slices.SortFunc(newBowl, func(a BowlEntry, b BowlEntry) int {
		return strings.Compare(a.Path, b.Path)
	})

	return newBowl
}

func writeBowl(bowl []BowlEntry) {
	slices.SortFunc(bowl, func(a, b BowlEntry) int {
		return strings.Compare(a.Path, b.Path)
	})
	var bowlLines []string
	for _, bowlEntry := range bowl {
		bowlLines = append(bowlLines, fmt.Sprintf("%s %s", bowlEntry.Object.Hash, bowlEntry.Path))
	}
	content := strings.Join(bowlLines, "\n")
	var buf bytes.Buffer
	buf.Write([]byte(content))
	writeFile(BOWL_PATH, buf)
}

func findFileInWorkdir(path string) os.FileInfo {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return info
}

func getObject(hash string) Object {
	var objectPath = fmt.Sprintf(OBJECTS_PATH+"/%s", hash)
	var reader, err = os.Open(objectPath)
	if err != nil {
		panic(err)
	}

	objectBytes := decompress(reader)
	header := getHeader(objectBytes)

	contentBytes := objectBytes[header.Len:]
	content := string(contentBytes)

	return Object{Hash: hash, Header: header, Content: content, Bytes: objectBytes}

}

func createObject(objectType string, content string) Object {
	header, bytes := addHeader(objectType, content)
	hash := hash(bytes)
	objectPath := fmt.Sprintf(OBJECTS_PATH+"/%s", hash)

	writeFile(objectPath, compress(bytes))
	return Object{Hash: hash, Header: header, Bytes: bytes}
}

func getHeader(object []byte) Header {
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
	return Header{ObjectType: headerLines[0], Len: headerLen}
}

// Returns the header, and a byte array containing the full object content including the header
func addHeader(objectType string, objectContent string) (Header, []byte) {
	// Object format:

	// type
	// <empty-line>
	// content

	headerContent := objectType + "\n\n"
	header := Header{ObjectType: objectType, Len: len(headerContent), Content: headerContent}

	return header, []byte(headerContent + objectContent)
}

func getTree(hash string) Tree {
	fmt.Println(hash)
	return getObject(hash).ToTree()
}

// Generate trees from bowl entries
func createTree(bowlEntries []BowlEntry) Tree {
	nodes := []TreeNode{}
	bowlSubentryMap := make(map[string][]BowlEntry)

	for _, bowlEntry := range bowlEntries {
		dir, file := filepath.Split(bowlEntry.Path)
		if dir == "" {
			nodes = append(nodes, TreeNode{Name: file, NodeType: "file", Hash: bowlEntry.Object.Hash})
		} else {
			bowlEntry.Path = file
			bowlSubentryMap[dir] = append(bowlSubentryMap[dir], bowlEntry)
		}
	}

	// Create tree objects from subentries recursively
	for dir, bowlDirEntries := range bowlSubentryMap {
		subtree := createTree(bowlDirEntries)
		subtreeNode := TreeNode{Name: dir, NodeType: "tree", Hash: subtree.Object.Hash}
		nodes = append(nodes, subtreeNode)
	}

	slices.SortFunc(nodes, func(a TreeNode, b TreeNode) int {
		return strings.Compare(a.Name, b.Name)
	})

	treeEntries := []string{}
	for _, treeNode := range nodes {
		treeEntries = append(treeEntries, fmt.Sprintf("%s %s %s", treeNode.NodeType, treeNode.Hash, treeNode.Name))
	}

	var object = createObject("tree", strings.Join(treeEntries, "\n"))
	return object.ToTree()
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
