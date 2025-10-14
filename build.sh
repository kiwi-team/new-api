#!/bin/zsh

arch=$1
ldflags=$2
if [ -z "$arch" ]; then
    arch="amd64"
fi
go env -w GOOS=linux  
go env -w GOARCH=$arch
if [ -z "$ldflags" ]; then
    go build  -o oneapi main.go 
else
    #go build -ldflags "$ldflags"  -o oneapi main.go 
    go build -ldflags "-s -w"  -o oneapi main.go 
fi
go env -w GOOS=darwin 
go env -w GOARCH=arm64
