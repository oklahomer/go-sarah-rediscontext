FROM redis:6.2.7

ENV HOME /redis

COPY redis.conf ${HOME}/redis.conf

EXPOSE 6379

ENTRYPOINT exec redis-server ${HOME}/redis.conf
