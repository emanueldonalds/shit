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
)

type Action int64

const (
	ADD Action = iota
	FLUSH
)

type Command struct {
	action Action
	args   []string
}

func main() {
	var command *Command = parse_args()

	switch command.action {
	case ADD:
		add(command.args)
	}
}

func parse_args() *Command {
	if len(os.Args) < 2 {
		exit()
	}

	var action Action

	//Arg 1 is the action
	switch os.Args[1] {
	case "add":
		action = ADD
	case "flush":
		action = FLUSH
	default:
		exit()
	}

	return &Command{action: action, args: os.Args[2:]}
}

func add(args []string) {
    if len(args) < 1 {
        exit()
    }

    var in_file_path = args[0]
    var file_content string = read_file(in_file_path)
    var content_bytes []byte = add_file_header(in_file_path, file_content)
    var compressed = compress(content_bytes)

    var out_file_path = fmt.Sprintf(".shit/objects/%s", hash(content_bytes))
    write_file(out_file_path, compressed)
}

func read_file(path string) string {
    var bytes, err = os.ReadFile(path)

    if err != nil {
        panic(err)
    }
    return string(bytes)
}

func add_file_header(file_path string, content string) []byte {
    // Object format:
    //
    // type file
    // file path
    // file content

    return []byte(strings.Join([]string {"type file", file_path, content}, "\n"))
}

func compress(b []byte) bytes.Buffer {
    var buf bytes.Buffer
    w := zlib.NewWriter(&buf)
    w.Write(b)
    w.Close()
    return buf
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

func hash(bytes []byte) string {
    hasher := sha1.New()
    hasher.Write(bytes)
    return hex.EncodeToString(hasher.Sum(nil))
}

func exit() {
	fmt.Println("Usage:\n\nshit add <filename>\tAdd a file to the the index\nshit flush <filename>\tWrite the current index to a commit")
	os.Exit(1)
}
