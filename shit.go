package main

import "fmt"
import "os"

type Action int64

const (
    ADD Action = iota
    FLUSH
)

type Command struct {
    action Action
    args []string
}

func main() {
    var _ = parse_args()
}

func parse_args() string {
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

    fmt.Printf("Action is %d\n", action)

    return "hi"
}

func exit() {
        fmt.Println("Usage:\n\nshit add <filename>\tAdd a file to the the index\nshit flush <filename>\tWrite the current index to a commit")
        os.Exit(1)
}
