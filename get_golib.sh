#!/bin/sh
if [ ! -d ../golib ]; then
    cd ..
    echo "clone https://github.com/CHESSComputing/golib.git"
    git clone https://github.com/CHESSComputing/golib.git
    ls
    go mod init github.com/CHESSComputing/golib
    go mod tidy
    ls
    cd -
fi
