#!/bin/bash

home=$HOME

wget -O $home/btick https://github.com/udhos/btick/releases/download/v0.0/btick_linux_amd64-0.0

chmod a+rwx $home/btick

CACHE_REAL=d DB_REAL=dbreal DB_USER=dbuser DB_PASS=dbpass DB_HOST=dbhost DB_NAME=dbname $home/btick 2>&1 >>$home/btick.log &
