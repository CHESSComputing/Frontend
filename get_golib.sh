#!/bin/sh
if [ ! -d ../golib ]; then
    cd ..
    echo "clone https://github.com/CHESSComputing/golib.git"
    git clone https://github.com/CHESSComputing/golib.git
    latestTag=`git tag --list | tail -1`
    go get https://github.com/CHESSComputing/golib@${latestTag}
    cd -
fi
