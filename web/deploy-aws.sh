# #!/bin/zsh

#!/bin/zsh
host=161.189.239.3
ssh www@$host << eeooff
rm -rf /data/www/one-api/web/dist/*
mkdir -p /data/www/one-api/web/dist/
eeooff
echo delete_old_file_1

rsync -vrz  ./dist/* www@$host:/data/www/one-api/web/dist/
