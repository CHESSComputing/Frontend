#!/bin/sh
if [ ! -d ../golib ]; then
    wdir=$PWD
    cd ..
    echo "clone https://github.com/CHESSComputing/golib.git"
    git clone https://github.com/CHESSComputing/golib.git

    # checkout all dependencies based on last tag
    cd golib
    lastTag=$(git tag --list | tail -1)
    echo "lastTag=$lastTag"
    cd -

    cd $wdir
    for pkg in $(ls -l ../golib | grep ^d | awk '{print $9}'); do
      echo "go get github.com/CHESSComputing/golib/$pkg@$lastTag"
    done | /bin/sh
fi
