package main

import "fmt"
import "os"

func main() {
    var _ = parse_args()
}

func parse_args() string {
    if len(os.Args) < 2 {
        fmt.Println("Usage:\n\nshit add <filename>\tAdd a file to the the index\nshit flush <filename>\tWrite the current index to a commit")
        os.Exit(1)
    }

    //Arg 1 is the action
    var action = os.Args[1]
    fmt.Printf("Action is %s\n", action)

    for i, arg := range(os.Args[2:]) {
        fmt.Printf("Arg %d is %s\n", i, arg)
    }

    return "hi"
}
