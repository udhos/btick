#!/bin/sh

[ -z "$GOPATH" ] && export GOPATH=$HOME/go

echo GOPATH=$GOPATH

go get github.com/go-sql-driver/mysql
go get github.com/bradfitz/gomemcache/memcache
go get github.com/aws/aws-sdk-go

pkg=github.com/udhos/btick

gofmt -s -w main.go
go tool fix main.go
go tool vet .
[ -x $GOPATH/bin/gosimple ] && $GOPATH/bin/gosimple main.go
[ -x $GOPATH/bin/golint ] && $GOPATH/bin/golint main.go
[ -x $GOPATH/bin/staticcheck ] && $GOPATH/bin/staticcheck main.go
go test $pkg
go install -v $pkg
