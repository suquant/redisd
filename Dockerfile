FROM alpine:edge
MAINTAINER George Kutsurua <g.kutsurua@gmail.com>

RUN apk update && apk upgrade &&\
	apk add redis &&\
	rm -rf /var/cache/apk/*

COPY kubernetes-redis /kubernetes-redis

EXPOSE 6379

ENTRYPOINT ["/usr/bin/redis-server"]
CMD ["--port", "6379"]
