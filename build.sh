#!/bin/zsh

arch=$1
if [ -z "$arch" ]; then
    arch="amd64"
fi
go env -w GOOS=linux  
go env -w GOARCH=$arch
#go build -ldflags "-s -w"  -o oneapi main.go 
go build  -o oneapi main.go 
go env -w GOOS=darwin 
go env -w GOARCH=arm64
