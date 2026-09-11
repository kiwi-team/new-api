# #!/bin/zsh

#!/bin/zsh
host=69.5.20.1
ssh www@$host << eeooff
rm -rf /data/www/one-api/web/dist/*
mkdir -p /data/www/one-api/web/dist/
eeooff
echo delete_old_file_1

rsync -vrz  ./dist/* www@$host:/data/www/one-api/web/dist/
