# #!/bin/zsh

#!/bin/zsh
#host=118.196.44.235
host=69.5.10.107
ssh www@$host << eeooff
rm -rf /data/www/one-api/web/dist/*
mkdir -p /data/www/one-api/web/dist/
eeooff
echo delete_old_file_1

rsync -vrz  ./dist/* www@$host:/data/www/one-api/web/dist/
