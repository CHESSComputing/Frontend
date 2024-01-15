#!/bin/sh
if [ ! -d ../golib ]; then
    cd ..
    echo "clone https://github.com/CHESSComputing/golib.git"
    git clone https://github.com/CHESSComputing/golib.git
    cd -
fi
