ARG ARCH="amd64"
ARG OS="linux"
FROM golang:1.18 AS compile
ARG ARCH
ARG OS
# Install NodeJS
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - \
    && apt-get install -yq nodejs build-essential
WORKDIR /build
COPY . .
RUN make build
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"
ARG ARCH
ARG OS
COPY --from=compile /build/prometheus        /bin/prometheus
COPY --from=compile /build/promtool          /bin/promtool
COPY --from=compile /build/documentation/examples/prometheus.yml  /etc/prometheus/prometheus.yml
COPY --from=compile /build/console_libraries/                     /usr/share/prometheus/console_libraries/
COPY --from=compile /build/consoles/                              /usr/share/prometheus/consoles/
COPY --from=compile /build/LICENSE                                /LICENSE
COPY --from=compile /build/NOTICE                                 /NOTICE
COPY --from=compile /build/npm_licenses.tar.bz2                   /npm_licenses.tar.bz2
WORKDIR /prometheus
RUN ln -s /usr/share/prometheus/console_libraries /usr/share/prometheus/consoles/ /etc/prometheus/ && \
    chown -R nobody:nobody /etc/prometheus /prometheus
USER       nobody
EXPOSE     9090
VOLUME     [ "/prometheus" ]
ENTRYPOINT [ "/bin/prometheus" ]
CMD        [ "--config.file=/etc/prometheus/prometheus.yml", \
             "--storage.tsdb.path=/prometheus", \
             "--web.console.libraries=/usr/share/prometheus/console_libraries", \
             "--web.console.templates=/usr/share/prometheus/consoles" ]
