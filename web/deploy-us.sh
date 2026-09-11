# #!/bin/zsh

# 部署到海外sg的ec2上
host=54.177.131.134
ssh www@$host << eeooff
rm -rf /data/www/newapi/web/dist/*
mkdir -p /data/www/newapi/web/dist/
eeooff
echo delete_old_file_1

rsync -vrz  ./dist/* www@$host:/data/www/newapi/web/dist/
