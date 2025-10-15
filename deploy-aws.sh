#!/bin/zsh

onlyone=$1
deploy_to_host() {
  local host=$1
  local instance_name=$2

  echo "Deploying to $host ($instance_name)"

  ssh www@$host << eeooff
rm /data/www/one-api/oneapi
eeooff
  echo "delete_oneapi on $instance_name"

  rsync -vz  ./oneapi www@$host:/data/www/one-api/
  ssh ec2-user@$host << eeooff
sudo rm -rf /data/www/one-api/logs/*
sudo systemctl daemon-reload
sudo systemctl restart oneapi
eeooff
  echo "${instance_name}_done!"
}

if [ "$onlyone" = "1" ]; then
  deploy_to_host 161.189.239.3 "newapi-1"
else
  deploy_to_host 161.189.239.3 "newapi-1"
  deploy_to_host 68.79.61.248 "newapi-2"
fi

