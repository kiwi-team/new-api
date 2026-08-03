#!/bin/zsh

onlyone=$1
deploy_to_host() {
  local host=$1
  local instance_name=$2

  echo "Deploying to $host ($instance_name)"

  ssh www@$host << eeooff
rm -f /data/www/one-api/oneapi-bak
cp /data/www/one-api/oneapi /data/www/one-api/oneapi-bak
rm /data/www/one-api/oneapi
eeooff
  echo "delete_oneapi on $instance_name"

  rsync -vz  ./oneapi www@$host:/data/www/one-api/
  ssh root@$host << eeooff
sudo rm -rf /data/www/one-api/logs/*
sudo systemctl daemon-reload
sudo systemctl restart oneapi
eeooff
  echo "${instance_name}_done!"
}

deploy_to_host 47.251.35.179 "yjd-mixrouter-1"
#deploy_to_host 69.5.10.107 "yjd-mixrouter-web"