#!/bin/sh

[ -z "$GOPATH" ] && export GOPATH=$HOME/go

echo GOPATH=$GOPATH

pkg=github.com/udhos/btick

gofmt -s -w main.go
go tool fix main.go
go tool vet .
[ -x $GOPATH/bin/gosimple ] && $GOPATH/bin/gosimple main.go
[ -x $GOPATH/bin/golint ] && $GOPATH/bin/golint main.go
[ -x $GOPATH/bin/staticcheck ] && $GOPATH/bin/staticcheck main.go
go test $pkg
go install -v $pkg
