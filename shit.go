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
const REFS_PATH = SHIT_PATH + "/refs"

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

func (object Object) ToFlush() Flush {
	lines := strings.Split(object.Content, "\n")
	tree := strings.Split(lines[0], " ")[1]
	parent := strings.Split(lines[1], " ")[1]
	date := strings.Split(lines[2], " ")[1]
	message := strings.Join(lines[4:], "\n")

	return Flush{Object: object, Date: date, ParentHash: parent, TreeHash: tree, Message: message}
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

func (tree Tree) ToBowl() []BowlEntry {
	var createEntries func(root string, tree Tree) []BowlEntry
	createEntries = func(root string, tree Tree) []BowlEntry {
		entries := []BowlEntry{}
		for _, node := range tree.Nodes {
			if node.NodeType == "file" {
				object := getObject(node.Hash)
				path := filepath.Join(root, node.Name)
				entries = append(entries, BowlEntry{object, path})
			}
			if node.NodeType == "tree" {
				tree := getObject(node.Hash).ToTree()
				treePath := filepath.Join(root, node.Name)
				entries = append(entries, createEntries(treePath, tree)...)
			}
		}
		return entries
	}
	return createEntries("./", tree)
}

type BowlEntry struct {
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

func (tree Tree) ToBowlEntries(root string) []BowlEntry {
	bowl := []BowlEntry{}
	for _, node := range tree.Nodes {
		if node.NodeType == "file" {
			object := getObject(node.Hash)
			bowl = addToBowl(bowl, BowlEntry{Object: object, Path: root})
		} else if node.NodeType == "tree" {
			subtree := getTree(node.Hash)
			bowl = addToBowl(bowl, subtree.ToBowlEntries(node.Name)...)
		}
	}
	return bowl
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
	case "sniff":
		cmdSniff()
	case "log":
		cmdLog()
	case "flush":
		cmdFlush(command.Args)
	case "create-tree":
		cmdCreateTree()
	case "plunge":
		cmdPlunge(command.Args)
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
	createFs("dir", REFS_PATH)
	createFs("file", filepath.Join(REFS_PATH, "master"))

    writeFile(HEAD_PATH, bytes.NewBuffer([]byte("refs/master")))
}

func cmdAdd(args []string) {
	if len(args) < 1 {
		exitUsage()
	}

	bowl := getBowl()
	workdir := getWorkdir()
	var addList []string

	if args[0] == "-A" {
		// Add workdir files to addlist
		addList = workdir

		// As well as all files from bowl
		for _, bowlEntry := range bowl {
			addList = append(addList, bowlEntry.Path)
		}
	} else {
		addList = append(addList, args...)
	}

	for _, addFile := range addList {
		var existingWdFile *string
		var oldBowlEntry *BowlEntry

		for _, wdFile := range workdir {
			if wdFile == addFile {
				existingWdFile = &wdFile
			}
		}
		for _, bowlEntry := range bowl {
			if bowlEntry.Path == addFile {
				oldBowlEntry = &bowlEntry
			}
		}
		if existingWdFile != nil {
			object := createObject("file", readFile(*existingWdFile))
			bowlEntry := BowlEntry{Object: object, Path: *existingWdFile}
			bowl = addToBowl(bowl, bowlEntry)
		}
		if existingWdFile == nil && oldBowlEntry != nil {
			bowl = removeFromBowl(bowl, addFile)
		}
		if existingWdFile == nil && oldBowlEntry == nil {
			panic(fmt.Sprintf("File with path %s not found in neither workdir or bowl", addFile))
		}
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

// TODO show added/modified/deleted instead of just printing the bowl
func cmdSniff() {
	bowl := getBowl()
	for _, entry := range bowl {
		fmt.Printf("%s %s\n", entry.Object.Hash, entry.Path)
	}
}

func cmdLog() {
	head := getHead()
	printLog(*head)
}

func printLog(flush Flush) {
	fmt.Println("Flush " + flush.Object.Hash)
	fmt.Println(flush.Object.Content)
	if flush.ParentHash == "" {
		return
	}
	printLog(getFlush(flush.ParentHash))
}

func cmdFlush(args []string) {
	if len(args) < 2 || args[0] != "-m" {
		fmt.Println("A message is required when flusing (-m <message>).")
		exitUsage()
	}

	bowl := getBowl()
	if len(bowl) == 0 {
		fmt.Println("Your bowl is empty, add files to bowl with \"flush add <filename>\" first.")
		exitUsage()
	}

	tree := createTree(bowl)
	parent := getHead()
	createFlush(tree, parent, args[1])
}

func cmdCreateTree() {
	bowl := getBowl()
	tree := createTree(bowl)
	fmt.Println("Created tree " + tree.Object.Hash)
}

func cmdPlunge(args []string) {
	if len(args) > 1 {
		exitUsage()
	}

	currentBowl := getBowl()
	head := getFlush(args[0])
	fmt.Println("Head is " + head.Object.Hash)
	tree := getTree(head.TreeHash)
	newBowl := tree.ToBowl()

	deleteWdFiles(currentBowl)
	writeTreeToWd("./", tree)
	writeBowl(newBowl)

	fmt.Println("Plunged out " + head.Object.Hash)
}

func dirIsTracked() bool {
	_, err := os.Stat(SHIT_PATH)
	return err == nil
}

func checkInit(action string) {
    dirIsTracked := dirIsTracked()
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

func writeTreeToWd(root string, tree Tree) {
	for _, node := range tree.Nodes {
		if node.NodeType == "file" {
			object := getObject(node.Hash)
			filename := filepath.Join(root, node.Name)
			os.WriteFile(filename, object.Bytes, 0644)
		}
		if node.NodeType == "tree" {
			subtree := getObject(node.Hash).ToTree()
			dirname := filepath.Join(root, node.Name)
			os.Mkdir(dirname, 0644)
			writeTreeToWd(dirname, subtree)
		}
	}
}

func deleteWdFiles(bowl []BowlEntry) {
	for _, bowlEntry := range bowl {
		pathParts := strings.Split(bowlEntry.Path, string(filepath.Separator))
		for i := len(pathParts); i > 0; i-- {
			nodePath := strings.Join(pathParts[:i], string(filepath.Separator))
			_, err := os.Stat(nodePath)
			if err != nil {
				continue
			}
			os.Remove(nodePath)
		}
	}
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
	headRef := string(headFile)
    head := getRef(headRef)
    if head == nil { // If no commit has yet been made, head is nil.
		return nil
	}
	return head
}

func getRef(name string) *Flush {
    refPath := filepath.Join(REFS_PATH, name)
    _, err := os.Stat(refPath)
    if err != nil {
        // Ref doesn't exist, which is the inital case for master ref if no commit is yet created.
        return nil
    }
    ref, err := os.ReadFile(refPath)
    if err != nil {
        panic(err)
    }
    flush := getFlush(string(ref))
    return &flush
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

	fmt.Println("Created flush " + flush.Hash)
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

func addToBowl(bowl []BowlEntry, newEntries ...BowlEntry) []BowlEntry {
	var newBowl []BowlEntry

	for _, oldEntry := range bowl {
		for _, newEntry := range newEntries {
			if oldEntry.Path != newEntry.Path {
				newBowl = append(newBowl, oldEntry)
			}
		}
	}

	newBowl = append(newBowl, newEntries...)
	slices.SortFunc(newBowl, func(a BowlEntry, b BowlEntry) int {
		return strings.Compare(a.Path, b.Path)
	})

	return newBowl
}

func removeFromBowl(bowl []BowlEntry, path string) []BowlEntry {
	var newBowl []BowlEntry
	for _, entry := range bowl {
		if entry.Path != path {
			newBowl = append(newBowl, entry)
		}
	}
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
    buf := bytes.NewBuffer([]byte(content))
	writeFile(BOWL_PATH, buf)
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
	bowlSubentryMap := make(map[string][]BowlEntry) // dirname -> subentries

	for _, bowlEntry := range bowlEntries {
		dir, file := filepath.Split(bowlEntry.Path)
		if dir == "" {
			nodes = append(nodes, TreeNode{Name: file, NodeType: "file", Hash: bowlEntry.Object.Hash})
		} else {
			bowlEntry.Path = file
			bowlSubentryMap[dir] = append(bowlSubentryMap[dir], bowlEntry)
		}
	}

	// Create tree objects from subentries
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

func writeFile(path string, buf *bytes.Buffer) {
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	_, err = io.Copy(file, buf)
	if err != nil {
		panic(err)
	}
}

func hash(bytes []byte) string {
	hasher := sha1.New()
	hasher.Write(bytes)
	return hex.EncodeToString(hasher.Sum(nil))
}

func compress(b []byte) *bytes.Buffer {
	var buf *bytes.Buffer
	w := zlib.NewWriter(buf)
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
	fmt.Fprint(w, "Usage:\n\n"+
		"shit init\tInitialize Shit repository\n"+
		"shit add <filename>\tAdd a file to the the bowl\n"+
		"shit sniff\tShow the current status of the bowl\n"+
		"shit log\tShow the flush logs\n"+
		"shit flush -m <message>\tWrite the current bowl to a flush\n"+
		"shit plunge <hash>\tPlunge out a specific flush\n")
	w.Flush()
	os.Exit(0)
}
