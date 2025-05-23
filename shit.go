package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

const SHIT_PATH = ".shit"
const OBJECTS_PATH = SHIT_PATH + "/objects"

type Command struct {
	action string
	args   []string
}

type Header struct {
	object_type string
	len         int
}

func main() {
	var command *Command = parse_args()

	if command.action == "--help" {
		exit_usage()
	}

	check_init(command.action)

	switch command.action {
	case "init":
		init_shit()
	case "add":
		add(command.args)
	case "get-object":
		get_object(command.args)
	default:
		exit_usage()
	}
}

func parse_args() *Command {
	if len(os.Args) < 2 {
		exit_usage()
	}

	return &Command{action: os.Args[1], args: os.Args[2:]}
}

func check_init(action string) {
	_, err := os.Stat(SHIT_PATH)
	dir_is_tracked := err == nil

	if !dir_is_tracked && action == "init" {
		return
	} else if dir_is_tracked && action == "init" {
		fmt.Println("Directory is already tracked by Shit, aborting init.")
		exit_usage()
	} else if !dir_is_tracked {
		fmt.Println("Directory is not tracked by Shit, initialize dir with \"shit init\" first.")
		exit_usage()
	}

}

func init_shit() {
	err := os.Mkdir(SHIT_PATH, 0775)
	if err != nil {
		panic(err)
	}
}

func add(args []string) {
	if len(args) < 1 {
		exit_usage()
	}

	var in_file_path = args[0]
	var file_content string = read_file(in_file_path)
	var content_bytes []byte = add_header(file_content)
	var compressed_object = compress(content_bytes)

	var object_path = fmt.Sprintf(OBJECTS_PATH+"/%s", hash(content_bytes))
	write_file(object_path, compressed_object)
}

func read_object(hash string) string {
	var object_path = fmt.Sprintf(OBJECTS_PATH+"/%s", hash)

	var object_reader, err = os.Open(object_path)
	if err != nil {
		panic(err)
	}

	buf := new(strings.Builder)

	r, err := zlib.NewReader(object_reader)
	if err != nil {
		panic(err)
	}

	io.Copy(buf, r)

	object_reader.Close()
	r.Close()

	return buf.String()
}

func get_object(args []string) {
	if len(args) < 1 {
		exit_usage()
	}

	hash := args[0]
	object := read_object(hash)
	object_type := get_object_type(object)
	var header_len int
	switch object_type {
	case "file":
		header_len = get_header(object).len
	default:
		panic("Unknown object type")
	}
	content := object[header_len:]

	fmt.Print(content)
}

func get_object_type(object string) string {
	buf := new(strings.Builder)
	for _, c := range object {
		if c == '\n' {
			return buf.String()
		}
		buf.WriteRune(c)
	}
	panic("Could not read object type")
}

func add_header(content string) []byte {
	// File object format:

	// type
	// <empty-line>
	// content

	var parts = strings.Join([]string{"file", "", content}, "\n")

	return []byte(parts)
}

func get_header(object string) *Header {
	var header_len int
	line := 0
	for i, c := range object {
		if line == 2 {
			header_len = i
			break
		}
		if c == '\n' {
			line = line + 1
		}
	}

	header_lines := strings.Split(object[:header_len], "\n")
	return &Header{object_type: header_lines[0], len: header_len}
}

func write_file(path string, buf bytes.Buffer) {
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

func read_file(path string) string {
	var bytes, err = os.ReadFile(path)

	if err != nil {
		panic(err)
	}
	return string(bytes)
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

func exit_usage() {
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "Usage:\n\nshit init\tInitialize Shit repository\nshit add <filename>\tAdd a file to the the index\nshit flush <filename>\tWrite the current index to a commit\nshit sniff\tShow the current status of the index\tshit get-object\tGet an object from the object store")
	w.Flush()
	os.Exit(0)
}
