FROM scratch
COPY fsprobe /probe/fsprobe
COPY allowed.txt /data/allowed.txt
COPY denied.txt /data/denied.txt
