FROM alpine:edge
MAINTAINER George Kutsurua <g.kutsurua@gmail.com>

RUN apk update && apk upgrade &&\
	apk add redis &&\
	rm -rf /var/cache/apk/*

EXPOSE 6379

ENTRYPOINT ["/usr/bin/redis-server"]
CMD [""]
