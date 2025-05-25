#!/bin/bash

rm -rf .shit

echo "Hi there" > test.txt
echo "Another line" >> test.txt

go run shit.go init
go run shit.go add test.txt

go run shit.go flush -m "Flush message"
