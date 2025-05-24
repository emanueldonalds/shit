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
	Path       string
	Bytes      []byte
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
		object := createFileObject(path)
		bowl = appendBowl(bowl, object)
	}

	writeBowl(bowl)
}

func cmdGetObject(args []string) {
	if len(args) < 1 {
		exitUsage()
	}

	hash := args[0]
	object := getObject(hash)
	objectType := getObjectType(object)
	var headerLen int

	switch objectType {
	case "file":
		headerLen = getHeader(object).Len
	default:
		panic("Unknown object type")
	}

	content := object[headerLen:]
	fmt.Print(content)
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

func getBowl() []*Object {
	bowlFile := readFile(BOWL_PATH)
	bowlLines := strings.Split(bowlFile, "\n")
	var bowl []*Object

	if bowlLines[0] == "" {
		return bowl
	}

	for _, line := range bowlLines {
		if line == "" {
			continue
		}

		lineParts := strings.Split(line, " ")
		bowl = append(bowl, &Object{ObjectType: lineParts[0], Hash: lineParts[1], Path: lineParts[2]})
	}

	return bowl
}

func writeBowl(bowl []*Object) {
	var buf bytes.Buffer
	for _, bowlObject := range bowl {
		buf.WriteString(fmt.Sprintf("%s %s %s\n", bowlObject.ObjectType, bowlObject.Hash, bowlObject.Path))
	}
	writeFile(BOWL_PATH, buf)
}

func appendBowl(bowl []*Object, newObject *Object) []*Object {
	var newBowl []*Object

	for _, oldObject := range bowl {
		if oldObject.Path != newObject.Path {
			newBowl = append(newBowl, oldObject)
		}
	}

	newBowl = append(newBowl, newObject)
	return newBowl
}

func getObject(hash string) string {
	var objectPath = fmt.Sprintf(OBJECTS_PATH+"/%s", hash)

	var objectReader, err = os.Open(objectPath)
	if err != nil {
		panic(err)
	}

	buf := new(strings.Builder)

	compressReader, err := zlib.NewReader(objectReader)
	if err != nil {
		panic(err)
	}

	io.Copy(buf, compressReader)
	objectReader.Close()
	compressReader.Close()

	return buf.String()
}

func createFileObject(path string) *Object {
	var fileContent string = readFile(path)
	var bytes []byte = addHeader("file", fileContent)
	var hash string = hash(bytes)
	var objectPath = fmt.Sprintf(OBJECTS_PATH+"/%s", hash)

	writeFile(objectPath, compress(bytes))
	return &Object{ObjectType: "file", Hash: hash, Path: path, Bytes: bytes}
}

func getObjectType(object string) string {
	buf := new(strings.Builder)
	for _, c := range object {
		if c == '\n' {
			return buf.String()
		}
		buf.WriteRune(c)
	}
	panic("Could not read object type")
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

func exitUsage() {
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "Usage:\n\nshit init\tInitialize Shit repository\nshit add <filename>\tAdd a file to the the index\nshit flush <filename>\tWrite the current index to a commit\nshit sniff\tShow the current status of the index\tshit get-object\tGet an object from the object store")
	w.Flush()
	os.Exit(0)
}
