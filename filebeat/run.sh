#!/bin/sh
df | grep rbd | awk '{print $1}' | xargs umount
for i in `df | awk '{ print $6}'|grep "contain.*shm"`;do umount  $i;done
hostname=$NODE_NAME
sed 's/${hostname}/'$hostname'/g' /etc/filebeat/config/beat-template.yml > beat.yml && \
	./filebeat --c beat.yml --path.data /usr/local/share/filebeat --path.logs /var/log/filebeat
