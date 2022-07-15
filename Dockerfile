FROM registry.access.redhat.com/ubi9/ubi
ARG TARGETARCH
COPY bin/connectopus-linux-${TARGETARCH} /usr/bin/connectopus
RUN chmod 0755 /usr/bin/connectopus
CMD ["/usr/bin/connectopus"]
