#!/bin/bash

rm -f .shit/objects/*

echo "Hi there" > test.txt
echo "Another line" >> test.txt


go run shit.go add test.txt

hash=$(ls .shit/objects)

rm test.txt

go run shit.go get-object "$hash" > test.txt
