#!/bin/sh
if [ ! -d ../golib ]; then
    cwd=$PWD
    cd ..
    echo "clone https://github.com/CHESSComputing/golib.git"
    git clone https://github.com/CHESSComputing/golib.git
    cd golib
    ls
    go mod init github.com/CHESSComputing/golib
    go mod tidy
    ls
    cd $cwd
fi
