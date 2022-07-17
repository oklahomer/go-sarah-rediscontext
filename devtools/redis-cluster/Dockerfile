FROM redis:6.2.7

ENV HOME /redis-cluster

COPY setup.sh ${HOME}/setup.sh

EXPOSE 7000 7001 7002 7003 7004 7005

ENTRYPOINT ${HOME}/setup.sh; sleep infinity
