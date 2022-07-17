#!/bin/bash

path=$(cd "dirname $0" || exit ;pwd)

# Set up Redis Cluster nodes
for i in $(seq 7000 7005); do
    nodeDir=$path/node-$i
    mkdir "$nodeDir"

    # https://redis.io/docs/manual/scaling/#creating-and-using-a-redis-cluster
    cat <<NODE_CONF > "${nodeDir}/redis.conf"
port $i
cluster-enabled yes
cluster-config-file nodes$i.conf
cluster-node-timeout 5000
appendonly no
NODE_CONF

    redis-server "${nodeDir}/redis.conf" --daemonize yes
done

# Set up Redis Cluster
redis-cli --cluster create \
    127.0.0.1:7000 \
    127.0.0.1:7001 \
    127.0.0.1:7002 \
    127.0.0.1:7003 \
    127.0.0.1:7004 \
    127.0.0.1:7005 \
    --cluster-replicas 1 \
    --cluster-yes
