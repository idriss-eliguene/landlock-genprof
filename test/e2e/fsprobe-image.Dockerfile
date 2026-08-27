FROM busybox:1.36.1
COPY fsprobe /probe/fsprobe
COPY allowed.txt /data/allowed.txt
COPY denied.txt /data/denied.txt
