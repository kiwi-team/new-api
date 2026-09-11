# #!/bin/zsh

#!/bin/zsh
#host=69.5.10.107
host=47.251.35.179
echo deploy_mixrouter_admin_web_to_$host
ssh www@$host << eeooff
rm -rf /data/www/one-api/web/dist/*
mkdir -p /data/www/one-api/web/dist/
eeooff
echo delete_old_file_1_done!

rsync -vrz  ./dist/* www@$host:/data/www/one-api/web/dist/


# host=69.5.22.106
# echo deploy_mixrouter_custom_web_to_$host
# ssh www@$host << eeooff
# rm -rf /data/www/one-api/web/dist/*
# mkdir -p /data/www/one-api/web/dist/
# eeooff
# echo delete_old_file_2_done!

#rsync -vrz  ./dist/* www@$host:/data/www/one-api/web/dist/