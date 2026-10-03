FROM chrislusf/seaweedfs:4.48@sha256:4e61d15fd35994cb1e43e1e553dff106794841fd9a99ade2fc8c8bfce4d7872d AS runtime
RUN apk upgrade --no-cache
COPY dockers/object-storage/entrypoint.sh /usr/local/bin/recorder-object-storage
RUN chmod 755 /usr/local/bin/recorder-object-storage
ENTRYPOINT ["/usr/local/bin/recorder-object-storage"]
