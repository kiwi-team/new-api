#!/bin/zsh

go env -w GOOS=linux  
go env -w GOARCH=amd64
#go build -ldflags "-s -w"  -o oneapi main.go 
go build  -o oneapi main.go 
go env -w GOOS=darwin 
go env -w GOARCH=arm64
