#!/bin/sh

cur_dir=$(cd $(dirname $0); pwd)

go build -o gmem

exec $cur_dir/gmem $cur_dir/conf/conf.json || echo "start failed"
